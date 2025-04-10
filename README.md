チュートリアルのリファクタリング案を以下の通りにまとめました。内容を整理し、手順を明確化し、再利用性や理解しやすさを高めました。

---

# Operator Go学習チュートリアル: Pod/Deployment管理のOperator作成

## 前提条件

このチュートリアルを実行するには、以下のソフトウェアが必要です：

```bash
# Operator SDKのインストール
export ARCH=$(case $(uname -m) in
  x86_64) echo -n amd64 ;;
  aarch64) echo -n arm64 ;;
  *) echo -n $(uname -m) ;;
esac)

export OS=$(uname | awk '{print tolower($0)}')

export OPERATOR_SDK_DL_URL="https://github.com/operator-framework/operator-sdk/releases/download/v1.31.0/operator-sdk_${OS}_${ARCH}"

curl -LO ${OPERATOR_SDK_DL_URL}

chmod +x operator-sdk_linux_amd64 && sudo mv operator-sdk_linux_amd64 /usr/local/bin/operator-sdk

# Goのインストール
wget https://go.dev/dl/go1.21.4.linux-amd64.tar.gz && sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.21.4.linux-amd64.tar.gz

# パス設定 (永続化)
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
echo 'export GOPATH=$HOME/go' >> ~/.bashrc
echo 'export PATH=$PATH:$GOPATH/bin' >> ~/.bashrc
source ~/.bashrc

# Makeのインストール
sudo apt-get update && sudo apt-get install -y make
```

## 1. 環境設定

まず、新しいディレクトリを作成し、Operator SDKを使ってプロジェクトを初期化します：

```bash
# 新しいディレクトリの作成
mkdir -p ~/dev/pod-manager-operator && cd ~/dev/pod-manager-operator

# Operator SDKでプロジェクトを初期化
operator-sdk init --plugins go/v3 --domain example.com --repo github.com/example/pod-manager-operator

# Go モジュールの依存関係を解決 (vet エラー対策)
go mod tidy
go mod download github.com/onsi/ginkgo/v2
```

次に、Kubernetesリソースを管理するAPIを作成します：

```bash
# APIの作成
operator-sdk create api --group example --version v1 --kind PodManager --resource --controller
```

## 2. カスタムリソース定義の設定

`api/v1/podmanager_types.go` ファイルを編集して、PodManagerの仕様を定義します：

```go
// PodManagerSpec defines the desired state of PodManager
type PodManagerSpec struct {
	Replicas      int32  `json:"replicas,omitempty"`
	RestartPolicy string `json:"restartPolicy,omitempty"`  // Always, OnFailure, Never
}

// PodManagerStatus defines the observed state of PodManager
type PodManagerStatus struct {
	AvailableReplicas int32  `json:"availableReplicas,omitempty"`
	Status            string `json:"status,omitempty"`
}
```

## 3. コントローラーの実装

`controllers/podmanager_controller.go` ファイルを編集して、Deploymentの管理ロジックを実装します：

```go
func (r *PodManagerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling PodManager", "namespacedName", req.NamespacedName)

	// PodManagerリソースの取得
	var podManager examplev1.PodManager
	if err := r.Get(ctx, req.NamespacedName, &podManager); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("PodManager resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get PodManager")
		return ctrl.Result{}, err
	}

	// Deploymentの取得または作成
	var deployment appsv1.Deployment
	err := r.Get(ctx, client.ObjectKey{Name: podManager.Name, Namespace: podManager.Namespace}, &deployment)
	if err != nil && errors.IsNotFound(err) {
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
		podManager.Status.AvailableReplicas = int32(len(pods.Items))
		podManager.Status.Status = "Running"
		if err := r.Status().Update(ctx, &podManager); err != nil {
			logger.Error(err, "Failed to update PodManager status")
			return ctrl.Result{}, err
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
						}}}},
					RestartPolicy: restartPolicy,
				},
			},
		},
	}

	// Controllerリファレンスの設定
	ctrl.SetControllerReference(m, dep, r.Scheme)
	return dep
}
```

## 4. マニフェストの生成とインストール

カスタムリソース定義（CRD）を生成し、Kubernetesクラスターにインストールします：

```bash
# CRDの生成
make manifests

# CRDのインストール
make install
```

## 5. Operator実行とリソース作成

別のターミナルでOperatorを実行します：

```bash
# Operatorの実行
make run
```

次に、PodManagerリソースを定義して適用します。以下の内容で`~/dev/k8s-ubuntu-kind-api-01-ingress-basic/k8s/podmanager.yaml`を作成します：

```yaml
apiVersion: example.example.com/v1
kind: PodManager
metadata:
  name: sample-podmanager
spec:
  replicas: 3
  restartPolicy: Always
```

そして、リソースを適用します：

```bash
kubectl apply -f ~/dev/k8s-ubuntu-kind-api-01-ingress-basic/k8s/podmanager.yaml
```

## 6. 動作確認

作成されたリソースとDeploymentを確認します：

```bash
# PodManagerリソースの確認
kubectl get podmanagers.example.example.com

# Deploymentの確認
kubectl get deployments

# Podの確認
kubectl get pods
```

## 7. 負荷テスト

無限ループでCPU負荷をかけるPodを作成します：

```bash
kubectl run load-generator --image=python:3.8 --command -- /bin/sh -c "while true; do echo 'hello'; done"
```

## まとめ

このチュートリアルでは、Operator SDKを使用してKubernetesオペレーターを作成し、カスタムリソースを管理する方法を学びました。実運用環境では、より詳細なエラーハンドリングやセキュリティ対策、テスト等を考慮する必要があります。