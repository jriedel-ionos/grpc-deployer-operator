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
	"fmt"

	operatorv1 "github.com/jriedel-ionos/grpc-deployer-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
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
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Operator")

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
		// reconcile delete
		if err := r.reconcileDelete(ctx, operator); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	if err := r.reconcileEnsure(ctx, operator); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OperatorReconciler) reconcileEnsure(ctx context.Context, operator *operatorv1.Operator) error {
	if controllerutil.AddFinalizer(operator, operatorv1.OperatorFinalizer) {
		if err := r.Update(ctx, operator); err != nil {
			return err
		}
	}

	if err := r.createOrUpdateServerDeployment(ctx, operator); err != nil {
		return err
	}

	if err := r.createOrUpdateServerService(ctx, operator); err != nil {
		return err
	}

	if err := r.createOrUpdateFrontendDeployment(ctx, operator); err != nil {
		return err
	}

	return r.createOrUpdateFrontendService(ctx, operator)
}

func (r *OperatorReconciler) reconcileDelete(ctx context.Context, operator *operatorv1.Operator) error {
	if err := r.deleteFrontendService(ctx, operator); err != nil {
		return err
	}

	if err := r.deleteFrontendDeployment(ctx, operator); err != nil {
		return err
	}

	if err := r.deleteServerService(ctx, operator); err != nil {
		return err
	}

	if err := r.deleteServerDeployment(ctx, operator); err != nil {
		return err
	}

	if controllerutil.RemoveFinalizer(operator, operatorv1.OperatorFinalizer) {
		if err := r.Update(ctx, operator); err != nil {
			return err
		}
	}

	return nil
}

func (r *OperatorReconciler) deleteServerDeployment(ctx context.Context, operator *operatorv1.Operator) error {
	key := client.ObjectKey{Namespace: operator.Namespace, Name: operator.Name + "-server"}
	deployment := &appsv1.Deployment{}

	if err := r.Get(ctx, key, deployment); err != nil {
		if errors.IsNotFound(err) {
			return err
		}
	}

	if controllerutil.RemoveFinalizer(deployment, operatorv1.OperatorFinalizer+"/server") {
		if err := r.Update(ctx, deployment); err != nil {
			return err
		}
	}

	err := r.Delete(ctx, deployment)
	if err != nil {
		return err
	}

	return nil
}

func (r *OperatorReconciler) deleteServerService(ctx context.Context, operator *operatorv1.Operator) error {
	key := client.ObjectKey{Namespace: operator.Namespace, Name: operator.Name + "-server"}
	service := &corev1.Service{}

	if err := r.Get(ctx, key, service); err != nil {
		if errors.IsNotFound(err) {
			return err
		}

		return nil
	}

	if controllerutil.RemoveFinalizer(service, operatorv1.OperatorFinalizer+"/server-service") {
		if err := r.Update(ctx, service); err != nil {
			return err
		}
	}

	err := r.Delete(ctx, service)
	if err != nil {
		return err
	}

	return nil
}

func (r *OperatorReconciler) deleteFrontendDeployment(ctx context.Context, operator *operatorv1.Operator) error {
	key := client.ObjectKey{Namespace: operator.Namespace, Name: operator.Name + "-frontend"}
	deployment := &appsv1.Deployment{}

	if err := r.Get(ctx, key, deployment); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}

		return err
	}

	if controllerutil.RemoveFinalizer(deployment, operatorv1.OperatorFinalizer+"/frontend") {
		if err := r.Update(ctx, deployment); err != nil {
			return err
		}
	}

	err := r.Delete(ctx, deployment)
	if err != nil {
		return err
	}

	return nil
}

func (r *OperatorReconciler) deleteFrontendService(ctx context.Context, operator *operatorv1.Operator) error {
	key := client.ObjectKey{Namespace: operator.Namespace, Name: operator.Name + "-frontend"}
	service := &corev1.Service{}

	if err := r.Get(ctx, key, service); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}

		return err
	}

	if controllerutil.RemoveFinalizer(service, operatorv1.OperatorFinalizer+"/frontend-service") {
		if err := r.Update(ctx, service); err != nil {
			return err
		}
	}

	err := r.Delete(ctx, service)
	if err != nil {
		return err
	}

	return nil
}

func (r *OperatorReconciler) createOrUpdateServerDeployment(ctx context.Context, operator *operatorv1.Operator) error {
	replicas := operator.Spec.Replicas

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operator.Name + "-server",
			Namespace: operator.Namespace,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		controllerutil.AddFinalizer(deployment, operatorv1.OperatorFinalizer+"/server")

		value := corev1.EnvVar{Name: "TEST"}

		value.Value = operator.Spec.ReturnValue

		deployment.Spec = appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": operator.Name + "-server",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": operator.Name + "-server",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "grpc-server",
							Image: "ghcr.io/jriedel-ionos/rampup-challenge-grpc/server:latest",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: []corev1.EnvVar{value},
						},
					},
				},
			},
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (r *OperatorReconciler) createOrUpdateServerService(ctx context.Context, operator *operatorv1.Operator) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operator.Name + "-server",
			Namespace: operator.Namespace,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		controllerutil.AddFinalizer(service, operatorv1.OperatorFinalizer+"/server-service")
		service.Spec = corev1.ServiceSpec{
			Selector: map[string]string{
				"app": operator.Name + "-server",
			},
			Ports: []corev1.ServicePort{
				{
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
				},
			},
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (r *OperatorReconciler) createOrUpdateFrontendDeployment(ctx context.Context,
	operator *operatorv1.Operator,
) error {
	replicas := operator.Spec.Replicas
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operator.Name + "-frontend",
			Namespace: operator.Namespace,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		controllerutil.AddFinalizer(deployment, operatorv1.OperatorFinalizer+"/frontend")
		deployment.Spec = appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": operator.Name + "-frontend",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": operator.Name + "-frontend",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "grpc-frontend",
							Image: "ghcr.io/jriedel-ionos/rampup-challenge-grpc/frontend:latest",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8081,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "TARGET",
									Value: fmt.Sprintf("%s.%s.svc.cluster.local:8080", operator.Name+"-server", operator.Namespace),
								},
							},
						},
					},
				},
			},
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (r *OperatorReconciler) createOrUpdateFrontendService(ctx context.Context, operator *operatorv1.Operator) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operator.Name + "-frontend",
			Namespace: operator.Namespace,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		controllerutil.AddFinalizer(service, operatorv1.OperatorFinalizer+"/frontend-service")
		service.Spec = corev1.ServiceSpec{
			Selector: map[string]string{
				"app": operator.Name + "-frontend",
			},
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				{
					Port:       8081,
					TargetPort: intstr.FromInt(8081),
				},
			},
		}
		return nil
	}); err != nil {
		return err
	}

	key := client.ObjectKeyFromObject(service)
	err := r.Client.Get(ctx, key, service)
	if err != nil {
		return err
	}

	return r.Client.Status().Update(ctx, operator)
}

// SetupWithManager sets up the controller with the Manager.
func (r *OperatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1.Operator{}).
		Complete(r)
}
