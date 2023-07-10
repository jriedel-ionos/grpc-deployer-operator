/*
Copyright 2023.

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

package controller

import (
	"context"
	v1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1 "grpc-deployer-operator/api/v1"
)

// OperatorReconciler reconciles a Operator object
type OperatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=operator.my.domain,resources=operators,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operator.my.domain,resources=operators/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operator.my.domain,resources=operators/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Operator object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *OperatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	operator := &operatorv1.Operator{}
	if err := r.Get(ctx, req.NamespacedName, operator); err != nil {
		if errors.IsNotFound(err) {
			// CR not found
			return ctrl.Result{}, nil
		}
		// some error
		return ctrl.Result{}, err
	}

	if !operator.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, operator)
	}

	deployment := v1.Deployment{}
	err := r.Get(ctx, req.NamespacedName, &deployment)
	if err != nil && errors.IsNotFound(err) {
		// deployment does not exist, create
		return r.createDeployment(ctx, operator)
	} else if err != nil {
		// error getting deployment
		return ctrl.Result{}, err
	}

	// deployment exists, check updates
	return r.updateDeployment(ctx, *operator, &deployment)
}

func (r *OperatorReconciler) handleDeletion(ctx context.Context, operator *operatorv1.Operator) (ctrl.Result, error) {
	deployment := v1.Deployment{}
	deploymentName := types.NamespacedName{
		Namespace: operator.Namespace,
		Name:      operator.Name,
	}

	err := r.Get(ctx, deploymentName, &deployment)
	if err != nil && errors.IsNotFound(err) {
		// deployment does not exist, do nothing
		return ctrl.Result{}, nil
	} else if err != nil {
		// error getting deployment
		return ctrl.Result{}, err
	}

	// delete it actually
	if err := r.Delete(ctx, &deployment); err != nil {
		// error deleting deployment
		return ctrl.Result{}, err
	}

	operator.SetFinalizers([]string{})
	if err := r.Update(ctx, operator); err != nil {
		// error updating CR
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OperatorReconciler) createDeployment(ctx context.Context, operator *operatorv1.Operator) (ctrl.Result, error) {
	deployment := v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: operator.Namespace,
			Name:      operator.Name,
		},
		Spec: v1.DeploymentSpec{
			Replicas: &operator.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": operator.Name,
				},
			},
			Template: v12.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": operator.Name,
					},
				},
				Spec: v12.PodSpec{
					Containers: []v12.Container{
						{
							Name:  "grpc-server",
							Image: "ghcr.io/jriedel-ionos/rampup-challenge-grpc/server:latest",
							Ports: []v12.ContainerPort{
								{
									ContainerPort: operator.Spec.Port,
								},
							},
							Env: []v12.EnvVar{
								{
									Name:  "TEST",
									Value: operator.Spec.ReturnValue,
								},
							},
						},
					},
				},
			},
		},
	}

	// set controller reference
	if err := ctrl.SetControllerReference(operator, &deployment, r.Scheme); err != nil {
		// error setting controller reference
		return ctrl.Result{}, err
	}

	// create deployment
	if err := r.Create(ctx, &deployment); err != nil {
		// error creating deployment
		return ctrl.Result{}, err
	}

	// mark CR as deployed
	operator.Status.Deployed = true
	if err := r.Status().Update(ctx, operator); err != nil {
		// error updating CR status
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OperatorReconciler) updateDeployment(ctx context.Context, operator operatorv1.Operator, deployment *v1.Deployment) (ctrl.Result, error) {
	if *deployment.Spec.Replicas != operator.Spec.Replicas {
		// update replica number
		deployment.Spec.Replicas = &operator.Spec.Replicas

		// update deployment
		if err := r.Update(ctx, deployment); err != nil {
			// error updating deployment
			return ctrl.Result{}, err
		}
	}

	if deployment.Spec.Template.Spec.Containers[0].Env[0].Value != operator.Spec.ReturnValue {
		// update return value
		deployment.Spec.Template.Spec.Containers[0].Env[0].Value = operator.Spec.ReturnValue

		// update deployment
		if err := r.Update(ctx, deployment); err != nil {
			// error updating deployment
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OperatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1.Operator{}).
		Complete(r)
}
