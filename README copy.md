# k8s-ubuntu-kind-api-01-ingress-basic

## ✅ Ubuntu 22.04 + kind + ECR + Ingress で Express API を動かす最小構成チュートリアル  
📁 パス前提：`~/dev/k8s-ubuntu-kind-api-01-ingress-basic`

---

## 📌 全体の流れ

1. **Docker イメージをビルド → ECR に Push（事前済）**
2. **kind クラスタを作成（ECR Pull 対応）**
3. **Ingress Controller を導入**
4. **Deployment / Service / Ingress を適用**
5. **curl で動作確認**

---

## 🔧 前提環境

- AMI: **Ubuntu Server 22.04 LTS**
- ストレージ：30GB（`/var/lib/docker`確保のため）
- インストール済：`Docker`, `kind`, `kubectl`, `AWS CLI`
- AWS CLI：`aws configure` 済み
- ECR に `k8s-api-sample` イメージが Push 済み

---

## 1️⃣ kind クラスタ作成（ECR 認証込み）

```bash
cd ~/dev/k8s-ubuntu-kind-api-01-ingress-basic

ECR_TOKEN=$(aws ecr get-login-password --region ap-northeast-1)

cat <<EOF > kind-cluster.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    extraPortMappings:
      - containerPort: 80
        hostPort: 80
containerdConfigPatches:
  - |-
    [plugins."io.containerd.grpc.v1.cri".registry]
      [plugins."io.containerd.grpc.v1.cri".registry.auths."503561449641.dkr.ecr.ap-northeast-1.amazonaws.com"]
        username = "AWS"
        password = "$ECR_TOKEN"
EOF

kind create cluster --config kind-cluster.yaml

kind get clusters
```

> 💡 ポート80を使うので、**EC2のセキュリティグループでポート80を開放**しておくこと。

---

## 2️⃣ Ingress Controller 導入

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.9.5/deploy/static/provider/kind/deploy.yaml

kubectl label node kind-control-plane ingress-ready=true

kubectl wait --namespace ingress-nginx \
  --for=condition=Ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=180s
```

---

## 3️⃣ Deployment / Service / Ingress 定義

📁 すべて `~/dev/k8s-ubuntu-kind-api-01-ingress-basic/k8s/` に配置

### `k8s/deployment.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: k8s-api-sample
  labels:
    app: k8s-api-sample
spec:
  replicas: 1
  selector:
    matchLabels:
      app: k8s-api-sample
  template:
    metadata:
      labels:
        app: k8s-api-sample
    spec:
      imagePullSecrets:
      - name: regcred
      containers:
      - name: k8s-api-sample
        image: 503561449641.dkr.ecr.ap-northeast-1.amazonaws.com/k8s-api-sample:latest
        imagePullPolicy: Always
        ports:
        - containerPort: 3000
        resources:
          requests:
            cpu: "100m"
            memory: "128Mi"
          limits:
            cpu: "200m"
            memory: "256Mi" 
```

### `k8s/service.yaml`

```yaml
apiVersion: v1
kind: Service
metadata:
  name: k8s-api-sample
spec:
  selector:
    app: k8s-api-sample
  ports:
    - protocol: TCP
      port: 80
      targetPort: 3000
  type: ClusterIP 
```

### `k8s/ingress.yaml`

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: k8s-api-sample
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  ingressClassName: nginx
  rules:
  - http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: k8s-api-sample
            port:
              number: 80 
```

---

## 4️⃣ デプロイ & 動作確認

```bash
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml
kubectl apply -f k8s/ingress.yaml

# 動作確認
kubectl logs -l app=k8s-api-sample
# => Express server running on port 3000
# => Hello World accessed

curl -v http://localhost/posts
# => {"message":"Hello World"}


```

```
kind delete cluster
kind get clusters
```

---

## ✅ まとめ

- ディレクトリ：`~/dev/k8s-ubuntu-kind-api-01-ingress-basic`
- kind + ECR 認証付き Pull 成功
- Express API（3000） → Service（80）→ Ingress（/）のルーティング確認
- EC2上でローカル確認完了

---

