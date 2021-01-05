package wrapper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/go-logr/logr"
	"k8s.io/api/autoscaling/v2beta2"
	k8serrros "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Controls whether to turn on the HPA for this workload.
	HPAEnable = "hpa.autoscaling.navigatorcloud.io/enable"
	// minReplicas is the lower limit for the number of replicas to which the autoscaler
	// can scale down.
	HPAMinReplicas = "hpa.autoscaling.navigatorcloud.io/minReplicas"
	// maxReplicas is the upper limit for the number of replicas to which the autoscaler
	// can scale up.
	HPAMaxReplicas = "hpa.autoscaling.navigatorcloud.io/maxReplicas"
	// metrics contains the specifications for which to use to calculate the desired replica
	// count(the maximum replica count across all metrics will be used).
	HPAMetrics = "hpa.autoscaling.navigatorcloud.io/metrics"
	// The scheme of `schedule-jobs` is similar with `crontab`, create HPA resource for the
	// workload regularly.
	HPAScheduleJobs = "hpa.autoscaling.navigatorcloud.io/schedule-jobs"
)

var (
	HPADefaultLabels = map[string]string{
		"managed-by": "hpa-operator",
	}
)

type hpaOperator struct {
	client         client.Client
	log            logr.Logger
	namespacedName types.NamespacedName
	annotations    map[string]string
	kind           string
	uid            types.UID
}

func NewHPAOperator(client client.Client, log logr.Logger, namespacedName types.NamespacedName, annotations map[string]string, kind string, uid types.UID) HPAOperator {
	return &hpaOperator{
		client:         client,
		log:            log,
		namespacedName: namespacedName,
		annotations:    annotations,
		kind:           kind,
		uid:            uid,
	}
}

type HPAOperator interface {
	DoHorizontalPodAutoscaler(ctx context.Context) (bool, error)
}

func (h *hpaOperator) DoHorizontalPodAutoscaler(ctx context.Context) (bool, error) {
	hpaLog := h.log.WithName("HorizontalPodAutoscaler").WithValues(h.kind, h.namespacedName)

	enable := false
	annotationHPAEnable := false
	if val, ok := h.annotations[HPAEnable]; ok {
		if val == "true" {
			enable = true
		}
		annotationHPAEnable = true
	}
	if !enable {
		hpaLog.Info("the HPA is disabled in the workload")
		// 存在对应的 annotation，但是其值不为 true，那么执行删除 HPA 操作
		if annotationHPAEnable {
			hpa := &v2beta2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      h.namespacedName.Name,
					Namespace: h.namespacedName.Namespace,
				},
			}
			err := h.client.Delete(ctx, hpa)
			if err != nil && !k8serrros.IsNotFound(err) {
				hpaLog.Error(err, "failed to delete the HPA")
			}
		}
		return false, nil
	}

	scheduleEnable := false
	if _, ok := h.annotations[HPAScheduleJobs]; ok {
		scheduleEnable = true
	}
	// (1) 处理定时 HPA 资源
	if scheduleEnable {

	} else {
		// (2) 创建普通 HPA 资源
		requeue, err := h.nonScheduleHPA(ctx, hpaLog)
		if err != nil {
			return requeue, err
		}
	}
	return false, nil
}

func (h *hpaOperator) nonScheduleHPA(ctx context.Context, hpaLog logr.Logger) (bool, error) {
	minReplicas, err := extractAnnotationIntValue(h.annotations, HPAMinReplicas)
	if err != nil {
		h.log.Error(err, "extractAnnotation minReplicas failed")
	}
	// 创建普通的 HPA 资源时，maxReplicas 是必选字段
	maxReplicas, err := extractAnnotationIntValue(h.annotations, HPAMaxReplicas)
	if err != nil {
		return false, fmt.Errorf("extractAnnotation maxReplicas failed: %v", err)
	}
	blockOwnerDeletion := true
	isController := true
	ref := metav1.OwnerReference{
		APIVersion:         "apps/v1",
		Kind:               h.kind,
		Name:               h.namespacedName.Name,
		UID:                h.uid,
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}

	hpa := &v2beta2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.namespacedName.Name,
			Namespace: h.namespacedName.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				ref,
			},
			Labels: HPADefaultLabels,
		},
		Spec: v2beta2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: v2beta2.CrossVersionObjectReference{
				Kind:       h.kind,
				Name:       h.namespacedName.Name,
				APIVersion: "apps/v1",
			},
			MaxReplicas: maxReplicas,
			Metrics:     make([]v2beta2.MetricSpec, 0),
		},
	}
	if minReplicas != 0 {
		hpa.Spec.MinReplicas = &minReplicas
	}
	metricsExist := false
	if metricsVal, ok := h.annotations[HPAMetrics]; ok {
		metricsExist = true
		err := json.Unmarshal([]byte(metricsVal), &hpa.Spec.Metrics)
		if err != nil {
			return false, fmt.Errorf("extractAnnotation metrics failed: %v", err)
		}
	}

	curHPA := &v2beta2.HorizontalPodAutoscaler{}
	err = h.client.Get(ctx, types.NamespacedName{
		Namespace: hpa.Namespace,
		Name:      hpa.Name,
	}, curHPA)
	if err != nil {
		if k8serrros.IsNotFound(err) {
			// create
			err = h.client.Create(ctx, hpa)
			if err != nil && !k8serrros.IsAlreadyExists(err) {
				// 重新入队列, 触发下一次的处理逻辑
				return true, fmt.Errorf("failed to create HPA: %v", err)
			}
			return false, nil
		}
		// 重新入队列, 触发下一次的处理逻辑
		return true, fmt.Errorf("failed to get HPA: %v", err)
	}
	// update
	needUpdate := false
	if metricsExist {
		if !reflect.DeepEqual(curHPA.Spec, hpa.Spec) {
			needUpdate = true
		}
	} else {
		if !reflect.DeepEqual(curHPA.Spec.MinReplicas, hpa.Spec.MinReplicas) || !reflect.DeepEqual(curHPA.Spec.MaxReplicas, hpa.Spec.MaxReplicas) {
			needUpdate = true
		}
	}
	if needUpdate {
		hpaLog.Info("Annotation is diff and need to update")
		err = h.client.Update(ctx, hpa)
		if err != nil {
			return true, fmt.Errorf("failed to update HPA: %v", err)
		}
	}
	return false, nil
}

func extractAnnotationIntValue(annotations map[string]string, annotationName string) (int32, error) {
	strValue, ok := annotations[annotationName]
	if !ok {
		return 0, errors.New(annotationName + " annotation is missing for workload")
	}
	int64Value, err := strconv.ParseInt(strValue, 10, 32)
	if err != nil {
		return 0, errors.New(annotationName + " value for workload is invalid: " + err.Error())
	}
	value := int32(int64Value)
	if value <= 0 {
		return 0, errors.New(annotationName + " value for workload should be positive number")
	}
	return value, nil
}
