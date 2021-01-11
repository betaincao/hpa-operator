/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/navigatorcloud/hpa-operator/pkg/wrapper"
	k8serrros "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	requeueAfterTime = 500 * time.Microsecond
)

// StatefulSetReconciler reconciles a StatefulSet object
type StatefulSetReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch

func (r *StatefulSetReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	statefulset := &appsv1.StatefulSet{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, statefulset)
	if err != nil {
		if k8serrros.IsNotFound(err) {
			// Object not found, return.
			// Created objects are automatically garbage collected.
			// For additional clean logic use finalizers
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request
		return ctrl.Result{Requeue: true, RequeueAfter: requeueAfterTime}, err
	}

	hpaOperator := wrapper.NewHPAOperator(r.Client, req.NamespacedName, statefulset.Annotations, "Statefulset", statefulset.UID)
	requeue, err := hpaOperator.DoHorizontalPodAutoscaler()
	if err != nil {
		klog.Error(fmt.Sprintf("DoHorizontalPodAutoscaler failed: %v and statefulset: %s", err, req.NamespacedName))
	}
	if requeue {
		return ctrl.Result{Requeue: true, RequeueAfter: requeueAfterTime}, err
	}

	return ctrl.Result{}, nil
}

func (r *StatefulSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.StatefulSet{}).
		Complete(r)
}
