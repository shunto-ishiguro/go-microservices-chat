# Phase 5: コンテナ化 + Kubernetes (Docker, EKS, Kustomize)

> **期間目安**: 約6-8週間
> **難易度**: ★★★★★（中級〜上級）

---

## 学習目標

本フェーズでは、全サービスを Docker コンテナ化し、Kubernetes (Amazon EKS) 上にデプロイする。Kustomize による環境分離、Probe 設計、HPA、NetworkPolicy など本番運用に必要な設定を学ぶ。

| # | 目標 | 詳細 |
|---|------|------|
| 1 | Docker でコンテナイメージを構築できる | マルチステージビルド、最適化、セキュリティ |
| 2 | docker-compose でローカル開発環境を構築できる | 全サービスのローカル起動 |
| 3 | Kubernetes の基礎概念を理解し操作できる | Pod, Deployment, Service, Namespace |
| 4 | 本番品質の K8s マニフェストを作成できる | Probe, HPA, NetworkPolicy, PDB |
| 5 | Kustomize で環境分離ができる | dev/staging/prod の overlays |
| 6 | Amazon EKS クラスターを構築・運用できる | IRSA, ALB Ingress, ECR |

---

## 前提知識

- **Phase 4 完了**: AWS マネージドサービスとの統合が動作していること
- Linux コマンドの基本操作
- YAML 構文の基本
- ネットワークの基礎（IP, ポート, DNS）
- AWS の基本概念（VPC, IAM, EC2）

---

## ステップ

### ステップ 1: Docker の基礎

Docker コンテナの概念を理解し、Go アプリケーションのコンテナイメージを構築する。

- [ ] Docker の基礎概念:

| 概念 | 説明 |
|------|------|
| イメージ | コンテナの設計図（レイヤー構造） |
| コンテナ | イメージから作られた実行中のインスタンス |
| Dockerfile | イメージのビルド手順を記述するファイル |
| レジストリ | イメージの保管・配布場所（DockerHub, ECR） |
| レイヤーキャッシュ | ビルド高速化のためのキャッシュ機構 |

- [ ] Dockerfile の基本命令（`FROM`, `COPY`, `RUN`, `CMD`, `EXPOSE`, `ENV`）
- [ ] マルチステージビルドによる Go アプリケーションのイメージ構築:

```dockerfile
# services/user-service/Dockerfile
# ==========================================
# ステージ 1: ビルド
# ==========================================
FROM golang:1.23-alpine AS builder

WORKDIR /app

# 依存関係を先にコピーしてキャッシュを活用
COPY go.mod go.sum ./
RUN go mod download

# ソースコードをコピーしてビルド
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /bin/server ./cmd/server

# ==========================================
# ステージ 2: 実行
# ==========================================
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /bin/server /bin/server

EXPOSE 8081 9081

USER nonroot:nonroot

ENTRYPOINT ["/bin/server"]
```

- [ ] `.dockerignore` ファイルの作成:

```
.git
.github
*.md
.env
.env.*
tmp/
vendor/
**/*_test.go
**/testdata/
```

- [ ] イメージサイズの最適化:

| テクニック | 説明 | 効果 |
|-----------|------|------|
| マルチステージビルド | ビルドツールを最終イメージに含めない | 大幅削減 |
| distroless/scratch | 最小ベースイメージの使用 | ~10MB 以下 |
| `-ldflags="-s -w"` | デバッグ情報の除去 | ~30% 削減 |
| `.dockerignore` | 不要ファイルのコピー排除 | ビルド高速化 |

- [ ] イメージのセキュリティ（nonroot ユーザー、脆弱性スキャン）
- [ ] Docker イメージのタグ戦略（semver, git SHA, latest）

**確認ポイント**: `docker build` でイメージがビルドされ、`docker run` でサービスが起動すること。

---

### ステップ 2: docker-compose によるローカル開発環境

docker-compose を使って全サービスとインフラをローカルで起動する。

- [ ] docker-compose の基礎（サービス定義、ネットワーク、ボリューム）
- [ ] 全サービスの docker-compose 定義:

```yaml
# docker-compose.yml
services:
  # --- インフラ ---
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: chatuser
      POSTGRES_PASSWORD: chatpass
      POSTGRES_DB: chatdb
    ports:
      - "5432:5432"
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U chatuser -d chatdb"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5

  localstack:
    image: localstack/localstack:latest
    ports:
      - "4566:4566"
    environment:
      - SERVICES=dynamodb,s3,sqs,sns,cognito
      - DEFAULT_REGION=ap-northeast-1
    volumes:
      - "./scripts/localstack:/etc/localstack/init/ready.d"

  # --- アプリケーション ---
  user-service:
    build:
      context: ./services/user-service
      dockerfile: Dockerfile
    ports:
      - "8081:8081"
      - "9081:9081"
    environment:
      - DB_HOST=postgres
      - REDIS_URL=redis://redis:6379
      - AWS_ENDPOINT=http://localstack:4566
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy

  chat-service:
    build:
      context: ./services/chat-service
      dockerfile: Dockerfile
    ports:
      - "8082:8082"
      - "9082:9082"
    environment:
      - DB_HOST=postgres
      - REDIS_URL=redis://redis:6379
      - USER_SERVICE_ADDR=user-service:9081
      - AWS_ENDPOINT=http://localstack:4566
    depends_on:
      postgres:
        condition: service_healthy
      user-service:
        condition: service_started

  realtime-service:
    build:
      context: ./services/realtime-service
      dockerfile: Dockerfile
    ports:
      - "8083:8083"
    environment:
      - REDIS_URL=redis://redis:6379
      - CHAT_SERVICE_ADDR=chat-service:9082
    depends_on:
      redis:
        condition: service_healthy

  api-gateway:
    build:
      context: ./services/api-gateway
      dockerfile: Dockerfile
    ports:
      - "8080:8080"
    environment:
      - USER_SERVICE_ADDR=user-service:9081
      - CHAT_SERVICE_ADDR=chat-service:9082
    depends_on:
      - user-service
      - chat-service

volumes:
  postgres-data:
```

- [ ] ホットリロードの設定（Air や Docker volumes によるコードのマウント）
- [ ] `docker compose up`, `docker compose down`, `docker compose logs` の操作
- [ ] ヘルスチェックと依存関係の管理

**確認ポイント**: `docker compose up` で全サービスが起動し、API Gateway 経由で全機能が動作すること。

---

### ステップ 3: Kubernetes の基礎概念

Kubernetes の核となるリソースと概念を理解する。

- [ ] Kubernetes とは何か（コンテナオーケストレーション）
- [ ] アーキテクチャの概要:

```
┌─────────────────────────────────────────────────────┐
│                  Control Plane                       │
│  ┌──────────┐ ┌──────────┐ ┌────────────┐          │
│  │ API      │ │ etcd     │ │ Controller │          │
│  │ Server   │ │          │ │ Manager    │          │
│  └──────────┘ └──────────┘ └────────────┘          │
│  ┌──────────┐                                       │
│  │Scheduler │                                       │
│  └──────────┘                                       │
└────────────────────────┬────────────────────────────┘
                         │
         ┌───────────────┼───────────────┐
         │               │               │
    ┌────┴────┐    ┌────┴────┐    ┌────┴────┐
    │ Node 1  │    │ Node 2  │    │ Node 3  │
    │┌───┐┌──┐│    │┌───┐┌──┐│    │┌───┐┌──┐│
    ││Pod││  ││    ││Pod││  ││    ││Pod││  ││
    │└───┘│  ││    │└───┘│  ││    │└───┘│  ││
    │┌───┐│  ││    │┌───┐│  ││    │┌───┐│  ││
    ││Pod│└──┘│    ││Pod│└──┘│    ││Pod│└──┘│
    │└───┘    │    │└───┘    │    │└───┘    │
    │ kubelet │    │ kubelet │    │ kubelet │
    │ kube-   │    │ kube-   │    │ kube-   │
    │ proxy   │    │ proxy   │    │ proxy   │
    └─────────┘    └─────────┘    └─────────┘
```

- [ ] 主要リソースの理解:

| リソース | 説明 | 用途 |
|----------|------|------|
| Pod | 最小デプロイ単位 | コンテナの実行 |
| Deployment | Pod のレプリカ管理 | ローリングアップデート |
| Service | Pod への安定したアクセス | サービスディスカバリ |
| ConfigMap | 設定データの管理 | 環境変数、設定ファイル |
| Secret | 機密データの管理 | パスワード、API キー |
| Namespace | リソースの論理的な分離 | 環境分離、チーム分離 |
| Ingress | 外部からのアクセス制御 | L7 ロードバランシング |

- [ ] 宣言的管理 vs 命令的管理の違い
- [ ] ラベルとセレクターの仕組み
- [ ] Kubernetes DNS（サービスディスカバリ）

**確認ポイント**: Kubernetes の主要コンポーネントと各リソースの役割を説明できること。

---

### ステップ 4: kubectl の基本操作

Kubernetes クラスターを操作するための kubectl コマンドを習得する。

- [ ] kubectl のインストールと設定（`~/.kube/config`）
- [ ] 基本操作:

| コマンド | 説明 |
|----------|------|
| `kubectl get pods` | Pod 一覧の取得 |
| `kubectl get deployments` | Deployment 一覧の取得 |
| `kubectl get services` | Service 一覧の取得 |
| `kubectl describe pod <name>` | Pod の詳細情報 |
| `kubectl logs <pod>` | Pod のログ表示 |
| `kubectl logs <pod> -f` | Pod のログをリアルタイム表示 |
| `kubectl exec -it <pod> -- /bin/sh` | Pod 内でコマンド実行 |
| `kubectl apply -f <file>` | マニフェストの適用 |
| `kubectl delete -f <file>` | リソースの削除 |
| `kubectl port-forward <pod> 8080:8080` | ポートフォワーディング |
| `kubectl top pods` | Pod のリソース使用量 |

- [ ] Namespace の操作（`-n` フラグ、コンテキスト切り替え）
- [ ] `kubectl explain` によるリソース仕様の確認
- [ ] ローカル Kubernetes 環境の構築（kind または minikube）:

```bash
# kind でローカルクラスター作成
kind create cluster --name chat-platform --config kind-config.yaml

# クラスターの確認
kubectl cluster-info
kubectl get nodes
```

**確認ポイント**: kubectl を使って Pod の作成、ログ確認、exec が行えること。

---

### ステップ 5: 各サービスの K8s マニフェスト作成

各マイクロサービスの Kubernetes マニフェスト（Deployment, Service, ConfigMap, Secret）を作成する。

- [ ] Deployment マニフェストの作成:

```yaml
# k8s/base/user-service/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: user-service
  labels:
    app: user-service
    version: v1
spec:
  replicas: 2
  selector:
    matchLabels:
      app: user-service
  template:
    metadata:
      labels:
        app: user-service
        version: v1
    spec:
      serviceAccountName: user-service
      containers:
        - name: user-service
          image: user-service:latest
          ports:
            - name: http
              containerPort: 8081
              protocol: TCP
            - name: grpc
              containerPort: 9081
              protocol: TCP
          envFrom:
            - configMapRef:
                name: user-service-config
            - secretRef:
                name: user-service-secrets
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 256Mi
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
            initialDelaySeconds: 10
            periodSeconds: 15
          readinessProbe:
            httpGet:
              path: /readyz
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
          startupProbe:
            httpGet:
              path: /healthz
              port: http
            failureThreshold: 30
            periodSeconds: 2
```

- [ ] Service マニフェストの作成:

```yaml
# k8s/base/user-service/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: user-service
  labels:
    app: user-service
spec:
  type: ClusterIP
  ports:
    - name: http
      port: 8081
      targetPort: http
      protocol: TCP
    - name: grpc
      port: 9081
      targetPort: grpc
      protocol: TCP
  selector:
    app: user-service
```

- [ ] ConfigMap の作成:

```yaml
# k8s/base/user-service/configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: user-service-config
data:
  APP_ENV: "production"
  LOG_LEVEL: "info"
  LOG_FORMAT: "json"
  HTTP_PORT: "8081"
  GRPC_PORT: "9081"
  DB_NAME: "chatdb"
  DB_SSLMODE: "require"
  AWS_REGION: "ap-northeast-1"
```

- [ ] Secret の管理（外部シークレット管理の概念）:

```yaml
# k8s/base/user-service/secret.yaml (テンプレート)
apiVersion: v1
kind: Secret
metadata:
  name: user-service-secrets
type: Opaque
stringData:
  DB_HOST: "placeholder"
  DB_USER: "placeholder"
  DB_PASSWORD: "placeholder"
```

- [ ] 各サービス（user-service, chat-service, realtime-service, media-service, notification-service, api-gateway）のマニフェスト作成
- [ ] Resource Requests と Limits の適切な設定

**確認ポイント**: `kubectl apply` で全サービスが Kubernetes 上にデプロイされ、正常に動作すること。

---

### ステップ 6: Probe 設計

ヘルスチェックのための Probe（Liveness, Readiness, Startup）を適切に設計・実装する。

- [ ] 各 Probe の役割:

| Probe | 目的 | 失敗時の動作 | チェック対象 |
|-------|------|-------------|-------------|
| Liveness Probe | コンテナが生きているか | コンテナを再起動 | デッドロック、ハング |
| Readiness Probe | トラフィックを受けられるか | Service から除外 | DB 接続、依存サービス |
| Startup Probe | 起動が完了したか | Liveness 判定を遅延 | 初期化処理 |

- [ ] Go アプリケーションでのヘルスチェックエンドポイント実装:

```go
// ヘルスチェックハンドラーの例
package handler

import (
    "context"
    "encoding/json"
    "net/http"
    "time"
)

type HealthChecker struct {
    db    *sql.DB
    redis *redis.Client
}

// /healthz - Liveness: アプリケーション自体が動作しているか
func (h *HealthChecker) LivenessHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "alive"})
}

// /readyz - Readiness: トラフィックを処理できる状態か
func (h *HealthChecker) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
    defer cancel()

    // DB 接続チェック
    if err := h.db.PingContext(ctx); err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]string{
            "status": "not ready",
            "reason": "database connection failed",
        })
        return
    }

    // Redis 接続チェック
    if err := h.redis.Ping(ctx).Err(); err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]string{
            "status": "not ready",
            "reason": "redis connection failed",
        })
        return
    }

    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}
```

- [ ] Probe のパラメータ設計:

| パラメータ | Liveness | Readiness | Startup |
|-----------|----------|-----------|---------|
| `initialDelaySeconds` | 10 | 5 | 0 |
| `periodSeconds` | 15 | 10 | 2 |
| `timeoutSeconds` | 3 | 3 | 3 |
| `failureThreshold` | 3 | 3 | 30 |
| `successThreshold` | 1 | 1 | 1 |

- [ ] gRPC ヘルスチェック（`grpc_health_v1`）の実装
- [ ] Probe の適切なパラメータチューニング

**確認ポイント**: 各 Probe が正しく動作し、DB 接続断時に Readiness Probe が失敗して Service から除外されること。

---

### ステップ 7: HPA (Horizontal Pod Autoscaler) の設定

負荷に応じて Pod 数を自動スケールする HPA を設定する。

- [ ] HPA の概念と仕組み:

```
              ┌──────────────────┐
              │ Metrics Server   │
              │ (CPU/Memory)     │
              └────────┬─────────┘
                       │ メトリクス収集
                       ▼
              ┌──────────────────┐
              │ HPA Controller   │
              │                  │
              │ 目標: CPU 70%    │
              │ min: 2, max: 10  │
              └────────┬─────────┘
                       │ レプリカ数調整
                       ▼
              ┌──────────────────┐
              │ Deployment       │
              │ replicas: N      │
              └──────────────────┘
```

- [ ] Metrics Server のインストール
- [ ] HPA マニフェストの作成:

```yaml
# k8s/base/user-service/hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: user-service
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: user-service
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
        - type: Pods
          value: 2
          periodSeconds: 60
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
        - type: Pods
          value: 1
          periodSeconds: 120
```

- [ ] 各サービスの HPA 設定:

| サービス | minReplicas | maxReplicas | CPU Target |
|----------|-------------|-------------|------------|
| api-gateway | 2 | 8 | 70% |
| user-service | 2 | 6 | 70% |
| chat-service | 2 | 8 | 70% |
| realtime-service | 2 | 10 | 60% |
| media-service | 1 | 4 | 70% |
| notification-service | 1 | 4 | 70% |

- [ ] スケーリングの動作確認（負荷テストで Pod 数が増減すること）

**確認ポイント**: 負荷をかけると HPA により Pod が自動的にスケールアウトし、負荷が下がるとスケールインすること。

---

### ステップ 8: NetworkPolicy の設定

Pod 間の通信を制御する NetworkPolicy を設定し、セキュリティを強化する。

- [ ] NetworkPolicy の概念（default deny + allowlist）
- [ ] デフォルト deny ポリシー:

```yaml
# k8s/base/network-policies/default-deny.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-all
spec:
  podSelector: {}
  policyTypes:
    - Ingress
    - Egress
```

- [ ] サービスごとの allowlist ポリシー:

```yaml
# k8s/base/network-policies/user-service.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: user-service-policy
spec:
  podSelector:
    matchLabels:
      app: user-service
  policyTypes:
    - Ingress
    - Egress
  ingress:
    # api-gateway と chat-service からの受信を許可
    - from:
        - podSelector:
            matchLabels:
              app: api-gateway
        - podSelector:
            matchLabels:
              app: chat-service
      ports:
        - port: 8081   # HTTP
          protocol: TCP
        - port: 9081   # gRPC
          protocol: TCP
  egress:
    # DynamoDB (AWS) への送信を許可
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
      ports:
        - port: 443
          protocol: TCP
    # DNS の許可
    - to:
        - namespaceSelector: {}
          podSelector:
            matchLabels:
              k8s-app: kube-dns
      ports:
        - port: 53
          protocol: UDP
        - port: 53
          protocol: TCP
```

- [ ] 通信マトリクスの設計:

| 送信元 → 送信先 | api-gateway | user-svc | chat-svc | realtime-svc | media-svc | notif-svc | Redis | DynamoDB |
|-----------------|-------------|----------|----------|-------------|-----------|-----------|-------|----------|
| api-gateway | - | gRPC | gRPC | - | REST | - | - | - |
| user-svc | - | - | - | - | - | - | - | HTTPS |
| chat-svc | - | gRPC | - | gRPC | - | - | - | HTTPS |
| realtime-svc | - | - | gRPC | - | - | - | Redis | - |
| notification-svc | - | - | - | - | - | - | - | HTTPS |

- [ ] NetworkPolicy の動作テスト（許可されていない通信がブロックされること）

**確認ポイント**: default deny 設定後、許可された通信のみが成功し、それ以外がブロックされること。

---

### ステップ 9: Kustomize による環境分離

Kustomize を使って dev/staging/prod の環境ごとにマニフェストをカスタマイズする。

- [ ] Kustomize の基本概念（base + overlays）
- [ ] ディレクトリ構成:

```
k8s/
├── base/
│   ├── kustomization.yaml
│   ├── namespace.yaml
│   ├── user-service/
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   ├── configmap.yaml
│   │   ├── hpa.yaml
│   │   └── kustomization.yaml
│   ├── chat-service/
│   │   └── ...
│   ├── realtime-service/
│   │   └── ...
│   ├── media-service/
│   │   └── ...
│   ├── notification-service/
│   │   └── ...
│   ├── api-gateway/
│   │   └── ...
│   └── network-policies/
│       └── ...
├── overlays/
│   ├── dev/
│   │   ├── kustomization.yaml
│   │   ├── patches/
│   │   │   ├── deployment-replicas.yaml
│   │   │   └── resource-limits.yaml
│   │   └── configmap-env.yaml
│   ├── staging/
│   │   ├── kustomization.yaml
│   │   └── patches/
│   │       └── ...
│   └── prod/
│       ├── kustomization.yaml
│       └── patches/
│           ├── deployment-replicas.yaml
│           ├── resource-limits.yaml
│           └── hpa-scaling.yaml
```

- [ ] base の `kustomization.yaml`:

```yaml
# k8s/base/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - namespace.yaml
  - user-service/
  - chat-service/
  - realtime-service/
  - media-service/
  - notification-service/
  - api-gateway/
  - network-policies/

commonLabels:
  project: chat-platform
```

- [ ] overlay の例（dev 環境）:

```yaml
# k8s/overlays/dev/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: chat-platform-dev

resources:
  - ../../base

patches:
  - path: patches/deployment-replicas.yaml
  - path: patches/resource-limits.yaml

configMapGenerator:
  - name: user-service-config
    behavior: merge
    literals:
      - APP_ENV=development
      - LOG_LEVEL=debug
      - AWS_ENDPOINT=http://localstack:4566

images:
  - name: user-service
    newName: 123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/user-service
    newTag: dev-latest
```

- [ ] 環境ごとの差分:

| 設定項目 | dev | staging | prod |
|----------|-----|---------|------|
| replicas | 1 | 2 | 3+ (HPA) |
| CPU request | 50m | 100m | 200m |
| Memory request | 64Mi | 128Mi | 256Mi |
| LOG_LEVEL | debug | info | info |
| HPA | 無効 | 有効 | 有効 |
| NetworkPolicy | 緩い | 本番同等 | 厳格 |
| PDB | なし | なし | あり |

- [ ] `kustomize build` でマニフェストを生成して確認
- [ ] `kubectl apply -k` でデプロイ

**確認ポイント**: `kustomize build k8s/overlays/dev` で環境ごとにカスタマイズされたマニフェストが生成されること。

---

### ステップ 10: Amazon EKS クラスターの構築と接続

本番環境用の Amazon EKS クラスターを構築する。

- [ ] EKS の概念（マネージド Kubernetes サービス）
- [ ] EKS クラスターの構成要素:

| コンポーネント | 説明 |
|---------------|------|
| EKS Control Plane | AWS が管理する Kubernetes マスター |
| Managed Node Group | EC2 インスタンスで構成されるワーカーノード |
| Fargate Profile | サーバーレスなノード（オプション） |
| VPC | クラスターのネットワーク |
| IAM Role | クラスターとノードの権限 |

- [ ] eksctl を使ったクラスター作成:

```bash
# eksctl でのクラスター作成例
eksctl create cluster \
  --name chat-platform \
  --region ap-northeast-1 \
  --version 1.29 \
  --nodegroup-name standard-workers \
  --node-type t3.medium \
  --nodes 3 \
  --nodes-min 2 \
  --nodes-max 5 \
  --managed
```

- [ ] kubeconfig の設定:

```bash
aws eks update-kubeconfig --name chat-platform --region ap-northeast-1
```

- [ ] Amazon ECR（Elastic Container Registry）へのイメージプッシュ:

```bash
# ECR へのログイン
aws ecr get-login-password --region ap-northeast-1 | \
  docker login --username AWS --password-stdin 123456789012.dkr.ecr.ap-northeast-1.amazonaws.com

# イメージのタグ付けとプッシュ
docker tag user-service:latest 123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/user-service:v1.0.0
docker push 123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/user-service:v1.0.0
```

- [ ] EKS クラスターへのマニフェスト適用

**確認ポイント**: EKS クラスターが正常に動作し、ECR からイメージをプルして Pod が起動すること。

---

### ステップ 11: IRSA (IAM Roles for Service Accounts) の設定

Kubernetes の ServiceAccount に IAM ロールを紐付け、Pod から AWS サービスにアクセスする。

- [ ] IRSA の概念と仕組み:

```
┌─────────────────────┐     Assume Role    ┌──────────────┐
│ Pod                  │ ──────────────→    │ IAM Role     │
│  └─ ServiceAccount   │    (OIDC)          │ (per-service)│
│     └─ annotation:   │                    │              │
│        eks.amazonaws  │                    │ Policy:      │
│        .com/role-arn  │                    │ - DynamoDB   │
│                       │                    │ - S3         │
└─────────────────────┘                    │ - SQS/SNS   │
                                            └──────────────┘
```

- [ ] OIDC プロバイダーの設定（EKS クラスター作成時に自動設定）
- [ ] サービスごとの IAM ロール作成:

| サービス | 必要な AWS 権限 |
|----------|----------------|
| user-service | DynamoDB (users テーブル), Cognito |
| chat-service | DynamoDB (chat テーブル), SNS (Publish) |
| media-service | S3 (GetObject, PutObject), DynamoDB |
| notification-service | SQS (ReceiveMessage, DeleteMessage), DynamoDB |

- [ ] ServiceAccount へのアノテーション:

```yaml
# k8s/base/user-service/serviceaccount.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: user-service
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/chat-platform-user-service
```

- [ ] 最小権限の原則に基づいた IAM ポリシーの作成

**確認ポイント**: Pod 内から AWS CLI を実行し、IRSA 経由で AWS サービスにアクセスできること。

---

### ステップ 12: Ingress (AWS ALB Ingress Controller) の設定

外部からのトラフィックを Kubernetes サービスにルーティングする Ingress を設定する。

- [ ] AWS Load Balancer Controller のインストール
- [ ] Ingress リソースの作成:

```yaml
# k8s/base/api-gateway/ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: chat-platform-ingress
  annotations:
    kubernetes.io/ingress.class: alb
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/target-type: ip
    alb.ingress.kubernetes.io/listen-ports: '[{"HTTPS":443}]'
    alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:ap-northeast-1:123456789012:certificate/xxx
    alb.ingress.kubernetes.io/healthcheck-path: /healthz
    alb.ingress.kubernetes.io/healthcheck-interval-seconds: "15"
spec:
  rules:
    - host: api.chat-platform.example.com
      http:
        paths:
          - path: /api/
            pathType: Prefix
            backend:
              service:
                name: api-gateway
                port:
                  number: 8080
          - path: /ws
            pathType: Prefix
            backend:
              service:
                name: realtime-service
                port:
                  number: 8083
```

- [ ] TLS 終端の設定（ACM 証明書）
- [ ] WebSocket 対応の設定（stickiness, idle timeout）
- [ ] パスベースルーティングの設計

**確認ポイント**: ALB 経由で外部からアプリケーションにアクセスでき、HTTPS で通信できること。

---

### ステップ 13: PodDisruptionBudget (prod)

本番環境での可用性を保証するために PodDisruptionBudget を設定する。

- [ ] PDB の概念（自発的な中断時の可用性保証）:

| シナリオ | PDB の効果 |
|----------|-----------|
| ノードのドレイン | 最小稼働数を維持しながら Pod を退避 |
| クラスターアップグレード | ローリング更新時の可用性保証 |
| ノードのスケールダウン | 必要な Pod 数が維持されることを保証 |

- [ ] PDB マニフェストの作成:

```yaml
# k8s/overlays/prod/pdb/user-service.yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: user-service-pdb
spec:
  minAvailable: 1       # または maxUnavailable: 1
  selector:
    matchLabels:
      app: user-service
```

- [ ] 各サービスの PDB 設定:

| サービス | minAvailable | 理由 |
|----------|-------------|------|
| api-gateway | 2 | フロントエンド、高可用性が必要 |
| user-service | 1 | ステートレス、基本サービス |
| chat-service | 1 | ステートレス、基本サービス |
| realtime-service | 2 | WebSocket 接続を保持、切断を最小化 |
| notification-service | 1 | 非同期処理、一時的な停止は許容 |

- [ ] PDB の動作確認（`kubectl drain` での動作テスト）

**確認ポイント**: ノードのドレイン時に PDB が機能し、最小稼働数が維持されること。

---

## 成果物

Phase 5 完了時に以下が動作していること:

- [x] 全サービスの Docker イメージが最適化されて構築できる
- [x] docker-compose でローカル開発環境が一発で起動する
- [x] Kubernetes マニフェストが整備されている（Deployment, Service, ConfigMap, Secret）
- [x] Liveness, Readiness, Startup Probe が適切に設定されている
- [x] HPA による自動スケーリングが動作する
- [x] NetworkPolicy でサービス間通信が制御されている
- [x] Kustomize で dev/staging/prod の環境分離ができている
- [x] Amazon EKS クラスター上で全サービスが稼働している
- [x] IRSA により Pod から AWS サービスに安全にアクセスできる
- [x] ALB Ingress 経由で外部からアクセスできる
- [x] PDB が本番環境に設定されている

### サービス構成図（Phase 5 完了時 - EKS 上）

```
                    Internet
                       │
                       ▼
              ┌────────────────┐
              │   AWS ALB      │
              │  (Ingress)     │
              └───────┬────────┘
                      │
  ┌───────────────────┼─────── EKS Cluster ──────────────────────┐
  │                   │                                           │
  │    ┌──────────────┼──────────────┐                           │
  │    │              ▼              │                           │
  │    │    ┌──────────────────┐     │                           │
  │    │    │   api-gateway    │     │                           │
  │    │    └────────┬─────────┘     │                           │
  │    │        gRPC │               │                           │
  │    │    ┌────────┼────────┐      │                           │
  │    │    ▼        ▼        ▼      │                           │
  │    │ ┌──────┐ ┌──────┐ ┌──────┐ │  ┌──────────┐ ┌────────┐ │
  │    │ │user  │ │chat  │ │real  │ │  │media     │ │notif   │ │
  │    │ │svc   │ │svc   │ │time  │ │  │svc       │ │svc     │ │
  │    │ └──┬───┘ └──┬───┘ └──┬───┘ │  └────┬─────┘ └───┬────┘ │
  │    │    │IRSA    │IRSA    │     │       │IRSA       │IRSA  │
  │    └────┼────────┼────────┼─────┘       │           │      │
  │         │        │        │             │           │      │
  └─────────┼────────┼────────┼─────────────┼───────────┼──────┘
            │        │        │             │           │
            ▼        ▼        ▼             ▼           ▼
      ┌──────────┐ ┌─────┐ ┌─────┐   ┌──────┐   ┌──────────┐
      │ DynamoDB │ │Redis│ │Redis│   │  S3  │   │ SQS/SNS  │
      └──────────┘ └─────┘ └─────┘   └──────┘   └──────────┘
                                          Cognito
```

---

## 学べる技術

| カテゴリ | 技術 | 用途 |
|----------|------|------|
| コンテナ | Docker | アプリケーションのコンテナ化 |
| ローカル開発 | docker-compose | マルチサービスのローカル起動 |
| オーケストレーション | Kubernetes | コンテナの管理・運用 |
| CLI | kubectl | Kubernetes クラスターの操作 |
| 構成管理 | Kustomize | 環境分離、マニフェスト管理 |
| マネージド K8s | Amazon EKS | Kubernetes のマネージドサービス |
| コンテナレジストリ | Amazon ECR | Docker イメージの管理 |
| IAM | IRSA | Pod レベルの AWS 認証 |
| ネットワーク | AWS ALB Ingress Controller | L7 ロードバランシング |

---

## 参考リソース

### 公式ドキュメント

| リソース | URL | 説明 |
|----------|-----|------|
| Kubernetes Documentation | https://kubernetes.io/docs/ | Kubernetes 公式ドキュメント |
| Docker Documentation | https://docs.docker.com/ | Docker 公式ドキュメント |
| EKS User Guide | https://docs.aws.amazon.com/eks/latest/userguide/ | Amazon EKS 公式ガイド |
| Kustomize | https://kustomize.io/ | Kustomize 公式サイト |
| kubectl Reference | https://kubernetes.io/docs/reference/kubectl/ | kubectl コマンドリファレンス |

### 書籍・コース

| リソース | 著者 | 説明 |
|----------|------|------|
| Kubernetes Up & Running | Brendan Burns 他 | Kubernetes の定番入門書 |
| Kubernetes in Action | Marko Lukša | 実践的な Kubernetes 解説書 |
| CKA/CKAD Study Guide | Benjamin Muschko | CKA/CKAD 試験対策書 |
| Docker Deep Dive | Nigel Poulton | Docker の包括的な解説書 |

### ツール

| ツール | 用途 |
|--------|------|
| kind | ローカル Kubernetes クラスター |
| minikube | ローカル Kubernetes クラスター（代替） |
| eksctl | EKS クラスターの作成・管理 |
| k9s | Kubernetes のターミナル UI |
| Lens | Kubernetes の GUI 管理ツール |
| kubectx / kubens | コンテキスト/Namespace の切り替え |
| stern | 複数 Pod のログ集約表示 |

---

## 認定試験との関連

Phase 5 は **CKA (Certified Kubernetes Administrator)** と **CKAD (Certified Kubernetes Application Developer)** の試験範囲に直結する。また、AWS SAA-C03 の EKS 関連トピックもカバーする。

### CKA 試験との対応

| CKA 試験ドメイン | 配点 | Phase 5 の対応トピック |
|-----------------|------|----------------------|
| **ストレージ (10%)** | 10% | PersistentVolume, PersistentVolumeClaim（EBS CSI Driver） |
| **トラブルシューティング (30%)** | 30% | kubectl logs, describe, exec によるデバッグ、Probe の失敗調査 |
| **ワークロードとスケジューリング (15%)** | 15% | Deployment, HPA, Resource Requests/Limits, Pod スケジューリング |
| **クラスターアーキテクチャ (25%)** | 25% | Control Plane / Node コンポーネント、etcd, RBAC |
| **サービスとネットワーキング (20%)** | 20% | Service, Ingress, NetworkPolicy, DNS |

### CKAD 試験との対応

| CKAD 試験ドメイン | 配点 | Phase 5 の対応トピック |
|------------------|------|----------------------|
| **アプリケーション設計とビルド (20%)** | 20% | Docker マルチステージビルド、CronJob、マルチコンテナ Pod |
| **アプリケーションのデプロイ (20%)** | 20% | Deployment 戦略、Kustomize、ローリングアップデート |
| **アプリケーションの可観測性とメンテナンス (15%)** | 15% | Probe (Liveness, Readiness, Startup)、ログ、リソースモニタリング |
| **アプリケーション環境、設定、セキュリティ (25%)** | 25% | ConfigMap, Secret, ServiceAccount, SecurityContext, NetworkPolicy |
| **サービスとネットワーキング (20%)** | 20% | Service, Ingress, NetworkPolicy |

### AWS SAA-C03 との対応

| Phase 5 トピック | SAA-C03 試験での出題ポイント |
|-----------------|---------------------------|
| EKS | コンテナワークロードの実行基盤の選択（EKS vs ECS vs Fargate） |
| ECR | コンテナイメージの管理、ライフサイクルポリシー |
| ALB | Application Load Balancer の設計、ターゲットグループ |
| IRSA | Pod レベルの IAM 権限管理 |
| VPC | EKS クラスターのネットワーク設計（サブネット、セキュリティグループ） |

### 具体的な試験トピック対応表

| Phase 5 ステップ | CKA | CKAD | SAA-C03 |
|-----------------|-----|------|---------|
| ステップ 1: Docker | - | アプリケーション設計 | - |
| ステップ 3-4: K8s 基礎/kubectl | クラスターアーキテクチャ | - | - |
| ステップ 5: マニフェスト作成 | ワークロード | アプリケーションデプロイ | - |
| ステップ 6: Probe 設計 | トラブルシューティング | 可観測性 | - |
| ステップ 7: HPA | ワークロード | - | - |
| ステップ 8: NetworkPolicy | ネットワーキング | セキュリティ | - |
| ステップ 9: Kustomize | - | デプロイ | - |
| ステップ 10: EKS | - | - | コンテナサービス |
| ステップ 11: IRSA | - | セキュリティ | IAM |
| ステップ 12: Ingress (ALB) | ネットワーキング | ネットワーキング | ALB |
| ステップ 13: PDB | ワークロード | - | - |

> **注**: Phase 5 は CKA/CKAD の試験範囲の約70-80% をカバーする。残りの範囲（RBAC の詳細、etcd のバックアップ、クラスターのアップグレードなど）は別途試験対策として補完する必要がある。

---

## 前のフェーズ

[Phase 4: AWS 統合](./phase-4.md)

## 次のフェーズ

Phase 5 が完了したら [Phase 6: Terraform + 可観測性 + CI/CD](./phase-6.md) に進む。
