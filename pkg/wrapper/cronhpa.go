package wrapper

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/navigatorcloud/hpa-operator/pkg/apis"
	"github.com/robfig/cron/v3"
	"k8s.io/api/autoscaling/v2beta2"
	k8serrros "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	CronHPAType = "hpa.autoscaling.navigatorcloud.io/hpa-type"
)

type CronHPA interface {
	AddJob(job apis.Job, hpa *v2beta2.HorizontalPodAutoscaler, client client.Client) error
	Start()
	Stop()
	Remove(job apis.Job, hpa *v2beta2.HorizontalPodAutoscaler)
}

type cronHPA struct {
	daemon   *cron.Cron
	jobs     map[string]jobDetail
	jobsLock *sync.Mutex
}

type jobDetail struct {
	job     apis.Job
	entryID cron.EntryID
}

func NewCronHPA() CronHPA {
	c := &cronHPA{
		daemon:   cron.New(cron.WithSeconds()),
		jobs:     make(map[string]jobDetail),
		jobsLock: &sync.Mutex{},
	}
	return c
}

// AddJob
func (c *cronHPA) AddJob(job apis.Job, hpa *v2beta2.HorizontalPodAutoscaler, client client.Client) error {
	c.jobsLock.Lock()
	defer c.jobsLock.Unlock()
	jobKey := buildJobKey(job, hpa)
	if detail, ok := c.jobs[jobKey]; !ok {
		cronHPAJob := newCronHPAJob(hpa, client)
		entryID, err := c.daemon.AddJob(job.Schedule, cronHPAJob)
		if err != nil {
			klog.Errorf("failed to add job to cron daemon: %v and resource is %s/%s", err, hpa.Namespace, hpa.Name)
			return err
		} else {
			c.jobs[jobKey] = jobDetail{
				job:     job,
				entryID: entryID,
			}
		}
	} else {
		// already exist and check the schedule is same
		if !reflect.DeepEqual(job, detail.job) {
			c.Remove(job, hpa)
			cronHPAJob := newCronHPAJob(hpa, client)
			entryID, err := c.daemon.AddJob(job.Schedule, cronHPAJob)
			if err != nil {
				klog.Errorf("failed to add job to cron daemon: %v and resource is %s/%s", err, hpa.Namespace, hpa.Name)
				return err
			} else {
				c.jobs[jobKey] = jobDetail{
					job:     job,
					entryID: entryID,
				}
			}
		}
	}
	klog.Infof("cronHPAJob(%s) successfully added", jobKey)
	return nil
}

func (c *cronHPA) Start() {
	c.daemon.Start()
}

func (c *cronHPA) Stop() {
	c.daemon.Stop()
}

func (c *cronHPA) Remove(job apis.Job, hpa *v2beta2.HorizontalPodAutoscaler) {
	c.jobsLock.Lock()
	defer c.jobsLock.Unlock()
	jobKey := buildJobKey(job, hpa)
	if detail, ok := c.jobs[jobKey]; ok {
		c.daemon.Remove(detail.entryID)
	}
	klog.Infof("cronHPAJob(%s) successfully removed", jobKey)
}

type cronHPAJob struct {
	hpa    *v2beta2.HorizontalPodAutoscaler
	client client.Client
}

func newCronHPAJob(hpa *v2beta2.HorizontalPodAutoscaler, client client.Client) *cronHPAJob {
	return &cronHPAJob{
		hpa:    hpa,
		client: client,
	}
}

func (cj *cronHPAJob) Run() {
	klog.Infof("cronHPA(%s/%s) is run once", cj.hpa.Namespace, cj.hpa.Name)
	curHPA := &v2beta2.HorizontalPodAutoscaler{}
	err := cj.client.Get(context.TODO(), types.NamespacedName{
		Namespace: cj.hpa.Namespace,
		Name:      cj.hpa.Name,
	}, curHPA)
	switch {
	case err == nil:
		// searched and update
		if curHPA.Spec.MaxReplicas != cj.hpa.Spec.MaxReplicas || curHPA.Annotations[CronHPAType] != cj.hpa.Annotations[CronHPAType] {
			curHPA.Spec.MaxReplicas = cj.hpa.Spec.MaxReplicas
			curHPA.Spec.MinReplicas = cj.hpa.Spec.MinReplicas
			curHPA.Annotations[CronHPAType] = cj.hpa.Annotations[CronHPAType]
			err := cj.client.Update(context.TODO(), curHPA)
			if err != nil {
				klog.Errorf("failed to update CronHPA: %v and resource is %s/%s", err, cj.hpa.Namespace, cj.hpa.Name)
				return
			}
			klog.Infof("update CronHPA successfully and %s/%s", cj.hpa.Namespace, cj.hpa.Name)
		}
	case k8serrros.IsNotFound(err):
		// search not and create
		err := cj.client.Create(context.TODO(), cj.hpa)
		if err != nil {
			klog.Errorf("failed to create CronHPA: %v and resource is %s/%s", err, cj.hpa.Namespace, cj.hpa.Name)
			return
		}
		klog.Infof("create CronHPA successfully and resource is %s/%s", cj.hpa.Namespace, cj.hpa.Name)
	default:
		klog.Errorf("get CronHPA: %v and resource is %s/%s", err, cj.hpa.Namespace, cj.hpa.Name)
	}
	return
}

func buildJobKey(job apis.Job, hpa *v2beta2.HorizontalPodAutoscaler) string {
	return fmt.Sprintf("%s/%s/%s", job.Type, hpa.Namespace, hpa.Name)
}
