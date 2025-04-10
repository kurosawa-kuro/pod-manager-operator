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

# パス設定
export PATH=$PATH:/usr/local/go/bin && export GOPATH=$HOME/go && export PATH=$PATH:$GOPATH/bin

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
```

次に、Kubernetesリソースを管理するAPIを作成します：

```bash
# APIの作成
operator-sdk create api --group example --version v1 --kind PodManager --resource --controller
```

## 2. カスタムリソース定義の設定

`api/v1/podmanager_types.go`ファイルを編集して、PodManagerの仕様を定義します：

```go
// PodManagerSpec defines the desired state of PodManager
type PodManagerSpec struct {
	// Replicas is the number of pods to run
	Replicas int32 `json:"replicas,omitempty"`
	
	// RestartPolicy defines the restart policy for pods
	// +kubebuilder:validation:Enum=Always;OnFailure;Never
	RestartPolicy string `json:"restartPolicy,omitempty"`
}

// PodManagerStatus defines the observed state of PodManager
type PodManagerStatus struct {
	// AvailableReplicas represents the number of available pods
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`
	
	// Status represents the current status of the PodManager
	Status string `json:"status,omitempty"`
}
```

## 3. コントローラーの実装

`controllers/podmanager_controller.go`ファイルを編集して、Deploymentの管理ロジックを実装します：

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

## 注意点と改善点

1. **APIグループの一貫性**: 本チュートリアルでは、`example.example.com`というAPIグループを使用しています。実環境では、組織の実際のドメインを使用することをお勧めします。

2. **パスの修正**: 別のプロジェクトディレクトリで操作する場合は、パスを適宜修正してください。

3. **KINDクラスターの作成**: Kubernetesクラスターがない場合は、以下のコマンドでKINDクラスターを作成できます：
   ```bash
   cd ~/dev/k8s-ubuntu-kind-api-01-ingress-basic && kind create cluster --config kind-cluster.yaml
   ```

4. **CRD定義の正確性**: CRDと実際のリソース定義が一致していることを確認してください。不一致がある場合は、再度`make manifests install`を実行してCRDを更新します。

## トラブルシューティング

Operatorが正常に動作しない場合、以下の点を確認して修正してください：

### 1. RBAC設定の確認

RBAC（Role-Based Access Control）の設定が不完全だと、OperatorがKubernetesリソースを適切に管理できません。以下の設定を`config/rbac/role.yaml`に追加して、必要な権限を与えます：

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: podmanager-operator-role
rules:
  - apiGroups: [""]
    resources: ["pods", "pods/log"]
    verbs: ["get", "list", "create", "update", "delete"]
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["get", "list", "create", "update", "delete"]
  - apiGroups: ["example.example.com"]
    resources: ["podmanagers"]
    verbs: ["get", "list", "create", "update", "delete"]
```

次に、`config/rbac/role_binding.yaml`に以下を追加して、RBACの設定を適用します：

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: podmanager-operator-role-binding
subjects:
  - kind: ServiceAccount
    name: default
    namespace: default
roleRef:
  kind: ClusterRole
  name: podmanager-operator-role
  apiGroup: rbac.authorization.k8s.io
```

これらのファイルを適用するには：

```bash
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/rbac/role_binding.yaml
```

### 2. エラーハンドリングの強化

コントローラーでのエラーハンドリングを強化するために、より詳細なログ出力を追加しましょう。`controllers/podmanager_controller.go`の例：

```go
// コントローラーのReconcileメソッド内で、エラーが発生した場合により詳細なログを追加
if err := r.Get(ctx, req.NamespacedName, &podManager); err != nil {
    if errors.IsNotFound(err) {
        logger.Info("PodManager resource not found. Ignoring since object must be deleted")
        return ctrl.Result{}, nil
    }
    logger.Error(err, "Failed to get PodManager", "namespace", req.Namespace, "name", req.Name)
    return ctrl.Result{}, err
}
```

### 3. クラスターの状態確認

Kubernetesクラスターの状態を確認するために、以下のコマンドを実行します：

```bash
# クラスターの状態確認
kubectl get nodes

# PodManagerリソースの確認
kubectl get podmanagers.example.example.com --all-namespaces

# CRDの確認
kubectl get crds | grep podmanager

# Deploymentの確認
kubectl get deployments --all-namespaces
```

### 4. Operatorのデバッグ

Operatorのログをデバッグするには、以下のコマンドを実行します：

```bash
# ローカルで実行している場合のログ確認
# make runの出力を確認

# クラスター内にデプロイしている場合のログ確認
kubectl logs -l control-plane=controller-manager -n <namespace>
```

### 5. APIグループの不一致の修正

初期化時のドメイン設定とCRDの適用に不一致がある場合、以下を確認します：

```bash
# 競合するCRDの削除
kubectl delete crd podmanagers.example.com

# 正しいAPIバージョンでCRDを再適用
make manifests install
```

また、PodManagerのYAMLファイルで正しいAPIバージョンを使用していることを確認してください：

```yaml
apiVersion: example.example.com/v1  # 正しいAPIグループを指定
kind: PodManager
# ...
```

### 6. 変更後の再デプロイ

コードを変更した後は、必ず以下の手順で再デプロイしてください：

```bash
# マニフェストの再生成
make manifests

# CRDの再インストール
make install

# Operatorの再実行
make run
```

## まとめ

このチュートリアルでは、Operator SDKを使用してKubernetesオペレーターを作成し、カスタムリソースを管理する方法を学びました。基本的な操作としてDeploymentのスケーリングとPodの再起動ポリシーの設定を実装しました。

実運用環境では、より詳細なエラーハンドリングやセキュリティ対策、テスト等を考慮する必要があります。また、上記のトラブルシューティング手順を参考に、問題が発生した際は適切に対処してください。