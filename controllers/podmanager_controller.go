/*
Copyright 2025.

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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	examplev1 "github.com/example/pod-manager-operator/api/v1"
)

// PodManagerReconciler reconciles a PodManager object
type PodManagerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=example.example.com,resources=podmanagers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=example.example.com,resources=podmanagers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=example.example.com,resources=podmanagers/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *PodManagerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling PodManager", "namespacedName", req.NamespacedName)

	// PodManagerリソースの取得
	var podManager examplev1.PodManager
	if err := r.Get(ctx, req.NamespacedName, &podManager); err != nil {
		if errors.IsNotFound(err) {
			// リクエストオブジェクトが見つからない場合、レコンサイルを続行しない
			logger.Info("PodManager resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// エラーを読み取り、レコンサイル
		logger.Error(err, "Failed to get PodManager")
		return ctrl.Result{}, err
	}

	// Deploymentの取得または作成
	var deployment appsv1.Deployment
	err := r.Get(ctx, client.ObjectKey{Name: podManager.Name, Namespace: podManager.Namespace}, &deployment)
	if err != nil && errors.IsNotFound(err) {
		// Deploymentが存在しない場合は作成
		logger.Info("Creating a new Deployment", "Deployment.Namespace", podManager.Namespace, "Deployment.Name", podManager.Name)
		deployment = *r.deploymentForPodManager(&podManager)
		err = r.Create(ctx, &deployment)
		if err != nil {
			logger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
			return ctrl.Result{}, err
		}
	} else if err != nil {
		logger.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	// Deploymentのスケーリング
	if *deployment.Spec.Replicas != podManager.Spec.Replicas {
		logger.Info("Scaling Deployment", "currentReplicas", *deployment.Spec.Replicas, "desiredReplicas", podManager.Spec.Replicas)
		deployment.Spec.Replicas = pointer.Int32Ptr(podManager.Spec.Replicas)
		err = r.Update(ctx, &deployment)
		if err != nil {
			logger.Error(err, "Failed to update Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
			return ctrl.Result{}, err
		}
	}

	// Podのリスタートポリシーの更新
	if podManager.Spec.RestartPolicy != "" {
		var pods corev1.PodList
		listOpts := []client.ListOption{
			client.InNamespace(podManager.Namespace),
			client.MatchingLabels(map[string]string{"app": podManager.Name}),
		}
		if err := r.List(ctx, &pods, listOpts...); err != nil {
			logger.Error(err, "Failed to list pods", "PodManager.Namespace", podManager.Namespace, "PodManager.Name", podManager.Name)
			return ctrl.Result{}, err
		}

		// ステータスの更新
		// 最新のリソースを取得してからステータスを更新する
		latestPodManager := &examplev1.PodManager{}
		if err := r.Get(ctx, req.NamespacedName, latestPodManager); err != nil {
			logger.Error(err, "Failed to re-fetch PodManager before status update")
			return ctrl.Result{}, err
		}
		latestPodManager.Status.AvailableReplicas = int32(len(pods.Items))
		latestPodManager.Status.Status = "Running"
		if err := r.Status().Update(ctx, latestPodManager); err != nil {
			logger.Error(err, "Failed to update PodManager status")
			// エラーが発生した場合、リキューして再試行する可能性があるため、Result{} を返す
			return ctrl.Result{Requeue: true}, err
		}
	}

	return ctrl.Result{}, nil
}

// deploymentForPodManager はPodManagerに基づいて新しいDeploymentを返します
func (r *PodManagerReconciler) deploymentForPodManager(m *examplev1.PodManager) *appsv1.Deployment {
	labels := map[string]string{"app": m.Name}

	// リスタートポリシーの設定（デフォルトはAlways）
	restartPolicy := corev1.RestartPolicyAlways
	if m.Spec.RestartPolicy != "" {
		restartPolicy = corev1.RestartPolicy(m.Spec.RestartPolicy)
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32Ptr(m.Spec.Replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: "nginx:latest",
						Name:  "nginx",
						Ports: []corev1.ContainerPort{{
							ContainerPort: 80,
							Name:          "http",
						}},
					}},
					RestartPolicy: restartPolicy,
				},
			},
		},
	}

	// Controllerリファレンスの設定
	ctrl.SetControllerReference(m, dep, r.Scheme)
	return dep
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodManagerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&examplev1.PodManager{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
