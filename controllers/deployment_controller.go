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

	"github.com/go-logr/logr"
	"github.com/navigatorcloud/hpa-operator/pkg/wrapper"
	appsv1 "k8s.io/api/apps/v1"
	k8serrros "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeploymentReconciler reconciles a Deployment object
type DeploymentReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch

func (r *DeploymentReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	deployment := &appsv1.Deployment{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, deployment)
	if err != nil {
		if k8serrros.IsNotFound(err) {
			// Object not found, return.
			// Created objects are automatically garbage collected.
			// For additional clean logic use finalizers
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request
		return ctrl.Result{}, err
	}

	hpaOperator := wrapper.NewHPAOperator(r.Client, req.NamespacedName, deployment.Annotations, "Deployment", deployment.UID)
	requeue, err := hpaOperator.DoHorizontalPodAutoscaler()
	if err != nil {
		klog.ErrorS(err, "DoHorizontalPodAutoscaler failed", "deployment", req.NamespacedName)
		if requeue {
			return ctrl.Result{}, err
		}
	}

	klog.InfoS("DoHorizontalPodAutoscaler successfully", "deployment", req.NamespacedName)
	return ctrl.Result{}, nil
}

func (r *DeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.Deployment{}).
		Complete(r)
}
