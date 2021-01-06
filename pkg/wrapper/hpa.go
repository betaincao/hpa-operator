package wrapper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"k8s.io/api/autoscaling/v2beta2"
	k8serrros "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
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
	namespacedName types.NamespacedName
	annotations    map[string]string
	kind           string
	uid            types.UID
}

func NewHPAOperator(client client.Client, namespacedName types.NamespacedName, annotations map[string]string, kind string, uid types.UID) HPAOperator {
	return &hpaOperator{
		client:         client,
		namespacedName: namespacedName,
		annotations:    annotations,
		kind:           kind,
		uid:            uid,
	}
}

type HPAOperator interface {
	DoHorizontalPodAutoscaler() (bool, error)
}

func (h *hpaOperator) DoHorizontalPodAutoscaler() (bool, error) {
	enable := false
	annotationHPAEnable := false

	if val, ok := h.annotations[HPAEnable]; ok {
		if val == "true" {
			enable = true
		}
		annotationHPAEnable = true
	}
	if !enable {
		klog.InfoS("The HPA is disabled in the workload", h.kind, h.namespacedName)
		if annotationHPAEnable {
			hpa := &v2beta2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      h.namespacedName.Name,
					Namespace: h.namespacedName.Namespace,
				},
			}
			err := h.client.Delete(context.TODO(), hpa)
			if err != nil && !k8serrros.IsNotFound(err) {
				klog.ErrorS(err, "Failed to delete the HPA", h.kind, h.namespacedName)
			}
		}
		return false, nil
	}

	scheduleEnable := false
	if _, ok := h.annotations[HPAScheduleJobs]; ok {
		scheduleEnable = true
	}

	if scheduleEnable {

	} else {
		requeue, err := h.nonScheduleHPA()
		if err != nil {
			return requeue, err
		}
	}
	return false, nil
}

// scheduleHPA
// Logic for handling schedule HPA
func (h *hpaOperator) scheduleHPA() (bool, error) {
	return true, nil
}

// nonScheduleHPA
// Logic for handling nonSchedule HPA
func (h *hpaOperator) nonScheduleHPA() (bool, error) {
	minReplicas, err := extractAnnotationIntValue(h.annotations, HPAMinReplicas)
	if err != nil {
		klog.ErrorS(err, "ExtractAnnotation minReplicas failed", h.kind, h.namespacedName)
	}
	// When creating the nonSchedulerHPA, maxReplicas is a required filed
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
	err = h.client.Get(context.TODO(), types.NamespacedName{
		Namespace: hpa.Namespace,
		Name:      hpa.Name,
	}, curHPA)
	if err != nil {
		if k8serrros.IsNotFound(err) {
			err = h.client.Create(context.TODO(), hpa)
			if err != nil && !k8serrros.IsAlreadyExists(err) {
				// Requeue, triggering the next processing logic
				return true, fmt.Errorf("failed to create HPA: %v", err)
			}
			return false, nil
		}
		// Requeue, triggering the next processing logic
		return true, fmt.Errorf("failed to get HPA: %v", err)
	}

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
		klog.InfoS("annotation is diff and need to update", h.kind, h.namespacedName)
		err = h.client.Update(context.TODO(), hpa)
		if err != nil {
			// Requeue, triggering the next processing logic
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
