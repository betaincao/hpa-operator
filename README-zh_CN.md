# HPA-Operator

[English](./README.md) | 简体中文

## HPA-operator 作用
只需要在 Deployment 和 Statefulset 的声明中指定 HPA 相关的信息，HPA 的运维工作完全交给 HPA-Operator 去完成。

HPA-Operator 支持以下特性
- 自动化为各种 Workloads 创建 HPA 资源
- 支持定时 HPA 的功能(指定时间点对副本数做扩容或者缩容操作)
- 支持将副本数扩容到指定数量

## 参数说明
#### workloads 声明中的参数列表
| Parameter                                        | Option | Description         |
| ---------------------------------------------    | ------ |-----------         |
| hpa.autoscaling.navigatorcloud.io/enable         | false  | Controls whether to turn on the HPA for this workload |
| hpa.autoscaling.navigatorcloud.io/minReplicas    | true   | minReplicas is the lower limit for the number of replicas to which the autoscaler can scale down. |
| hpa.autoscaling.navigatorcloud.io/maxReplicas    | true   | maxReplicas is the upper limit for the number of replicas to which the autoscaler can scale up. |
| hpa.autoscaling.navigatorcloud.io/metrics        | true   | metrics contains the specifications for which to use to calculate the desired replica count(the maximum replica count across all metrics will be used). |
| hpa.autoscaling.navigatorcloud.io/schedule-jobs  | true   | The scheme of `schedule` is similar with `crontab`, create HPA resource for the workload regularly. |

#### 参数说明
`hpa.autoscaling.navigatorcloud.io/metrics` 

参数是 json 字符串，其结构体参考 `autoscaling/v2beta2` 版本的结构体
```golang
// HorizontalPodAutoscalerSpec describes the desired functionality of the HorizontalPodAutoscaler.
type HorizontalPodAutoscalerSpec struct {
    // metrics contains the specifications for which to use to calculate the
    // desired replica count (the maximum replica count across all metrics will
    // be used).  The desired replica count is calculated multiplying the
    // ratio between the target value and the current value by the current
    // number of pods.  Ergo, metrics used must decrease as the pod count is
    // increased, and vice-versa.  See the individual metric source types for
    // more information about how each type of metric must respond.
    // If not set, the default metric will be set to 80% average CPU utilization.
    // +optional
    Metrics []MetricSpec `json:"metrics,omitempty" protobuf:"bytes,4,rep,name=metrics"`
}

// MetricSpec specifies how to scale based on a single metric
// (only `type` and one other matching field should be set at once).
type MetricSpec struct {
	// type is the type of metric source.  It should be one of "ContainerResource", "External",
	// "Object", "Pods" or "Resource", each mapping to a matching field in the object.
	// Note: "ContainerResource" type is available on when the feature-gate
	// HPAContainerMetrics is enabled
	Type MetricSourceType `json:"type" protobuf:"bytes,1,name=type"`

	// object refers to a metric describing a single kubernetes object
	// (for example, hits-per-second on an Ingress object).
	// +optional
	Object *ObjectMetricSource `json:"object,omitempty" protobuf:"bytes,2,opt,name=object"`
	// pods refers to a metric describing each pod in the current scale target
	// (for example, transactions-processed-per-second).  The values will be
	// averaged together before being compared to the target value.
	// +optional
	Pods *PodsMetricSource `json:"pods,omitempty" protobuf:"bytes,3,opt,name=pods"`
	// resource refers to a resource metric (such as those specified in
	// requests and limits) known to Kubernetes describing each pod in the
	// current scale target (e.g. CPU or memory). Such metrics are built in to
	// Kubernetes, and have special scaling options on top of those available
	// to normal per-pod metrics using the "pods" source.
	// +optional
	Resource *ResourceMetricSource `json:"resource,omitempty" protobuf:"bytes,4,opt,name=resource"`
	// container resource refers to a resource metric (such as those specified in
	// requests and limits) known to Kubernetes describing a single container in
	// each pod of the current scale target (e.g. CPU or memory). Such metrics are
	// built in to Kubernetes, and have special scaling options on top of those
	// available to normal per-pod metrics using the "pods" source.
	// This is an alpha feature and can be enabled by the HPAContainerMetrics feature flag.
	// +optional
	ContainerResource *ContainerResourceMetricSource `json:"containerResource,omitempty" protobuf:"bytes,7,opt,name=containerResource"`
	// external refers to a global metric that is not associated
	// with any Kubernetes object. It allows autoscaling based on information
	// coming from components running outside of cluster
	// (for example length of queue in cloud messaging service, or
	// QPS from loadbalancer running outside of cluster).
	// +optional
	External *ExternalMetricSource `json:"external,omitempty" protobuf:"bytes,5,opt,name=external"`
}
```

`hpa.autoscaling.navigatorcloud.io/schedule-jobs`

该参数用于控制在指定的时间扩容指定数量的副本数或者缩容指定数量的副本数，结构体描述如下
```golang
package apis

type JobType string

const (
	ScaleUp   JobType = "scale-up"
	ScaleDown JobType = "scale-down"
)

type ScheduleJobs struct {
	Jobs []Job `json:"scheduleJobs"`
}

type Job struct {
	//
	Type       JobType `json:"type"`
	Schedule   string  `json:"schedule"`
	TargetSize int32   `json:"targetSize"`
}
```
> 注意一旦使用了 `hpa.autoscaling.navigatorcloud.io/schedule-jobs` 时，以下参数不可使用
> - `hpa.autoscaling.navigatorcloud.io/minReplicas`
> - `hpa.autoscaling.navigatorcloud.io/maxReplicas`
> - `hpa.autoscaling.navigatorcloud.io/metrics`

## 例子
(一) 创建一个当 CPU 资源使用率大于 85% 时自动扩缩容，最小副本数为5，最大副本数为10
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hpa-example
  labels:
    app: hpa-example
  annotations:
    hpa.autoscaling.navigatorcloud.io/enable: "true"
    hpa.autoscaling.navigatorcloud.io/minReplicas: "5"
    hpa.autoscaling.navigatorcloud.io/maxReplicas: "10"
    hpa.autoscaling.navigatorcloud.io/metrics: '[{"type":"Resource","resource":{"name":"cpu","target":{"type":"Utilization","averageUtilization":85}}}]'
```

(二) 创建一个副本数一直都是10的 HPA 资源
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hpa-example
  labels:
    app: hpa-example
  annotations:
    hpa.autoscaling.navigatorcloud.io/enable: "true"
    hpa.autoscaling.navigatorcloud.io/maxReplicas: "10"
```

(三) 创建一个 每天10点扩容10个副本，每天15点缩容到5个副本
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hpa-example
  labels:
    app: hpa-example
  annotations:
    hpa.autoscaling.navigatorcloud.io/enable: "true"
    hpa.autoscaling.navigatorcloud.io/schedule-jobs: '{"scheduleJobs":[{"type":"scale-up","schedule":"0 0 */10 * * *","targetSize":10},{"type":"scale-down","schedule":"0 0 */15 * * *","targetSize":5}]}'
```