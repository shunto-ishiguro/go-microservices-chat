# ディレクトリ構成

## プロジェクト全体構成

Go Workspace（`go.work`）を使ったモノレポ構成。

```
go-microservices-chat/
├── go.work                          # Go Workspace 定義
├── go.work.sum
├── Makefile                         # プロジェクト全体のタスク定義
├── README.md
├── .github/
│   └── workflows/
│       ├── ci.yml                   # CI パイプライン
│       ├── cd-dev.yml               # dev 環境デプロイ
│       ├── cd-staging.yml           # staging 環境デプロイ
│       └── cd-prod.yml             # prod 環境デプロイ
│
├── docs/                            # 設計ドキュメント
│   ├── architecture/
│   ├── aws/
│   ├── kubernetes/
│   ├── terraform/
│   └── learning/
│
├── proto/                           # Protocol Buffers 定義（共有）
│   ├── buf.yaml                     # Buf 設定
│   ├── buf.gen.yaml                 # コード生成設定
│   ├── user/
│   │   └── v1/
│   │       └── user.proto
│   ├── chat/
│   │   └── v1/
│   │       └── chat.proto
│   ├── realtime/
│   │   └── v1/
│   │       └── realtime.proto
│   ├── notification/
│   │   └── v1/
│   │       └── notification.proto
│   └── media/
│       └── v1/
│           └── media.proto
│
├── pkg/                             # 共有パッケージ
│   ├── go.mod
│   ├── go.sum
│   ├── logger/                      # 構造化ログ (slog)
│   │   └── logger.go
│   ├── middleware/                   # 共通ミドルウェア
│   │   ├── auth.go                  # JWT 検証
│   │   ├── logging.go               # リクエストログ
│   │   ├── recovery.go              # パニックリカバリ
│   │   └── cors.go                  # CORS 設定
│   ├── config/                      # 設定管理
│   │   └── config.go
│   ├── errors/                      # 共通エラー型
│   │   └── errors.go
│   ├── pagination/                  # ページネーション
│   │   └── cursor.go
│   └── testutil/                    # テストユーティリティ
│       ├── db.go                    # テスト用 DB ヘルパー
│       └── grpc.go                  # テスト用 gRPC ヘルパー
│
├── services/                        # マイクロサービス群
│   ├── user-service/
│   ├── chat-service/
│   ├── realtime-service/
│   ├── notification-service/
│   ├── media-service/
│   └── api-gateway/
│
├── terraform/                       # Infrastructure as Code
│   ├── modules/
│   └── environments/
│
├── kubernetes/                      # Kubernetes マニフェスト
│   ├── base/
│   └── overlays/
│
├── scripts/                         # ユーティリティスクリプト
│   ├── setup-local.sh               # ローカル環境セットアップ
│   ├── migrate.sh                   # DB マイグレーション
│   └── generate-proto.sh            # Proto コード生成
│
└── docker-compose.yml               # ローカル開発用
```

## サービスの内部構成

各マイクロサービスは以下の統一的な構成に従う。

```
services/user-service/
├── go.mod                           # サービス固有の依存管理
├── go.sum
├── Dockerfile                       # マルチステージビルド
├── Makefile                         # サービス固有タスク
├── cmd/
│   └── server/
│       └── main.go                  # エントリーポイント
├── internal/                        # 非公開パッケージ（外部インポート不可）
│   ├── config/
│   │   └── config.go                # 環境変数・設定読み込み
│   ├── domain/                      # ドメインモデル
│   │   ├── user.go                  # User エンティティ
│   │   └── friendship.go            # Friendship エンティティ
│   ├── repository/                  # データアクセス層
│   │   ├── repository.go            # Repository インターフェース
│   │   ├── postgres.go              # PostgreSQL 実装
│   │   └── dynamodb.go              # DynamoDB 実装 (Phase 4)
│   ├── service/                     # ビジネスロジック層
│   │   ├── user_service.go
│   │   └── user_service_test.go
│   ├── handler/                     # トランスポート層
│   │   ├── rest/                    # REST ハンドラー
│   │   │   ├── handler.go
│   │   │   ├── user_handler.go
│   │   │   └── user_handler_test.go
│   │   └── grpc/                    # gRPC ハンドラー
│   │       ├── server.go
│   │       ├── user_server.go
│   │       └── user_server_test.go
│   └── gen/                         # 自動生成コード（proto）
│       └── user/
│           └── v1/
│               ├── user.pb.go
│               └── user_grpc.pb.go
├── migrations/                      # DB マイグレーション
│   ├── 001_create_users.up.sql
│   ├── 001_create_users.down.sql
│   ├── 002_create_friendships.up.sql
│   └── 002_create_friendships.down.sql
└── tests/                           # 統合テスト・E2Eテスト
    ├── integration/
    │   └── user_test.go
    └── e2e/
        └── user_e2e_test.go
```

## レイヤードアーキテクチャ

各サービスは以下の層で構成される。

```
┌─────────────────────────────────┐
│          Handler 層              │  ← HTTP/gRPC リクエスト処理
│   (REST handler, gRPC server)    │     リクエストの変換・バリデーション
├─────────────────────────────────┤
│          Service 層              │  ← ビジネスロジック
│   (ユースケース・ドメインルール)    │     トランスポートに非依存
├─────────────────────────────────┤
│        Repository 層             │  ← データアクセス
│   (PostgreSQL, DynamoDB, Redis)  │     インターフェースで抽象化
└─────────────────────────────────┘
```

**依存の方向**: Handler → Service → Repository（上から下へ）

## Terraform ディレクトリ構成

```
terraform/
├── modules/                         # 再利用可能なモジュール
│   ├── networking/                  # VPC, Subnet, NAT Gateway
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   └── outputs.tf
│   ├── eks/                         # EKS クラスター
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   └── outputs.tf
│   ├── database/                    # RDS (PostgreSQL), DynamoDB
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   └── outputs.tf
│   ├── messaging/                   # SQS, SNS
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   └── outputs.tf
│   ├── storage/                     # S3, ECR
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   └── outputs.tf
│   ├── auth/                        # Cognito
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   └── outputs.tf
│   ├── cache/                       # ElastiCache (Redis)
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   └── outputs.tf
│   └── observability/               # CloudWatch, X-Ray
│       ├── main.tf
│       ├── variables.tf
│       └── outputs.tf
├── environments/
│   ├── dev/
│   │   ├── main.tf                  # モジュール呼び出し
│   │   ├── variables.tf
│   │   ├── terraform.tfvars         # dev 固有の値
│   │   ├── backend.tf               # S3 + DynamoDB バックエンド
│   │   └── outputs.tf
│   ├── staging/
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   ├── terraform.tfvars
│   │   ├── backend.tf
│   │   └── outputs.tf
│   └── prod/
│       ├── main.tf
│       ├── variables.tf
│       ├── terraform.tfvars
│       ├── backend.tf
│       └── outputs.tf
└── global/                          # 環境横断のリソース
    ├── s3-backend/                  # Terraform バックエンド用 S3/DynamoDB
    │   └── main.tf
    └── ecr/                         # ECR リポジトリ
        └── main.tf
```

## Kubernetes ディレクトリ構成

Kustomize を使った環境分離。

```
kubernetes/
├── base/                            # 共通マニフェスト
│   ├── kustomization.yaml
│   ├── namespace.yaml
│   ├── user-service/
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   └── hpa.yaml
│   ├── chat-service/
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   └── hpa.yaml
│   ├── realtime-service/
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   └── hpa.yaml
│   ├── notification-service/
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   └── hpa.yaml
│   ├── media-service/
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   └── hpa.yaml
│   ├── api-gateway/
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   ├── ingress.yaml
│   │   └── hpa.yaml
│   └── network-policies/
│       └── default-deny.yaml
├── overlays/
│   ├── dev/
│   │   ├── kustomization.yaml      # dev 用パッチ
│   │   ├── replicas-patch.yaml      # レプリカ数: 1
│   │   └── resources-patch.yaml     # リソース制限（小）
│   ├── staging/
│   │   ├── kustomization.yaml
│   │   ├── replicas-patch.yaml      # レプリカ数: 2
│   │   └── resources-patch.yaml     # リソース制限（中）
│   └── prod/
│       ├── kustomization.yaml
│       ├── replicas-patch.yaml      # レプリカ数: 3+
│       ├── resources-patch.yaml     # リソース制限（大）
│       └── pdb.yaml                 # PodDisruptionBudget
└── monitoring/                      # 可観測性
    ├── prometheus/
    │   └── service-monitor.yaml
    └── grafana/
        └── dashboards/
```

## go.work 設定

```go
go 1.23

use (
    ./pkg
    ./services/user-service
    ./services/chat-service
    ./services/realtime-service
    ./services/notification-service
    ./services/media-service
    ./services/api-gateway
)
```

## 関連ドキュメント

- [Terraform 構成詳細](../terraform/structure.md)
- [Kubernetes アーキテクチャ](../kubernetes/architecture.md)
