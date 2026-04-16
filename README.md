# Go Microservices Chat

リアルタイムチャットプラットフォームを **Go + AWS + Kubernetes** で構築するマイクロサービス学習プロジェクト。

## アーキテクチャ

```mermaid
graph TD
    Client["クライアント"] -->|HTTPS| GW["API Gateway<br/>:8080"]
    Client -->|WSS| GW

    GW -->|gRPC| US["User Service<br/>:50051 / :8001"]
    GW -->|gRPC| CS["Chat Service<br/>:50052"]
    GW -->|gRPC| NS["Notification Service<br/>:50053"]
    GW -->|gRPC| MS["Media Service<br/>:50054"]
    GW -->|WebSocket| RS["Realtime Service<br/>:50055 / :8081"]

    US --> PG1[("PostgreSQL")]
    CS --> DDB1[("DynamoDB")]
    NS --> DDB2[("DynamoDB")]
    MS --> S3[("S3")]
    RS --> Redis[("Redis")]
```

## サービス一覧

| サービス | 役割 | プロトコル | データストア |
|---------|------|-----------|------------|
| **user-service** | ユーザー管理・フレンド機能 | REST + gRPC | PostgreSQL |
| **chat-service** | チャットルーム・メッセージ管理 | gRPC | DynamoDB |
| **realtime-service** | WebSocket 接続・リアルタイム配信 | WebSocket + gRPC Streaming | Redis |
| **notification-service** | 通知管理・プッシュ配信 | gRPC | DynamoDB |
| **media-service** | ファイルアップロード・画像処理 | REST + gRPC | S3 |
| **api-gateway** | 認証・ルーティング・レート制限 | REST → gRPC 変換 | - |

## 技術スタック

| カテゴリ | 技術 |
|---------|------|
| 言語 | Go 1.22 |
| HTTP ルーター | Chi v5 |
| RPC | gRPC + Protocol Buffers (Buf CLI) |
| DB ドライバー | pgx v5 |
| ログ | log/slog |
| コンテナ | Docker / Kubernetes |
| IaC | Terraform |
| CI/CD | GitHub Actions |
| 認証 | Amazon Cognito |
| メッセージング | Amazon SQS / SNS |

## プロジェクト構成

```
go-microservices-chat/
├── services/           # マイクロサービス群
│   └── user-service/   #   ユーザー管理 (実装済み)
├── proto/              # Protocol Buffers 定義
│   └── user/v1/        #   UserService proto (定義済み)
├── gen/go/             # protobuf 生成コード
├── pkg/                # 共有パッケージ (errors, logger, middleware)
├── docs/               # 設計ドキュメント
├── docker-compose.yml  # ローカル開発用 DB
└── go.work             # Go Workspace
```

## セットアップ

### 前提条件

- Go 1.22+
- Docker / Docker Compose
- [golang-migrate](https://github.com/golang-migrate/migrate) CLI
- [Buf CLI](https://buf.build/docs/installation) (proto 関連のみ)

### 起動

```bash
# 1. PostgreSQL を起動
docker compose up -d

# 2. マイグレーション実行
migrate -path services/user-service/migrations \
  -database "postgres://chat:chat@localhost:5432/userdb?sslmode=disable" up

# 3. user-service を起動
go run ./services/user-service/cmd/server
```

### テスト

```bash
# user-service のテスト (DB 不要)
go test ./services/user-service/...
```

### Proto 生成

```bash
cd proto && buf generate
```

## 開発フェーズ

| Phase | 内容 | 状態 |
|-------|------|------|
| 1 | Go 基礎 - REST API (user-service + PostgreSQL) | **完了** |
| 2 | gRPC + サービス間通信 | Step 1 完了 (Proto 定義) |
| 3 | Kubernetes デプロイ | - |
| 4 | AWS サービス統合 (DynamoDB, SQS, S3) | - |
| 5 | 可観測性 + CI/CD | - |
| 6 | 本番運用 + セキュリティ | - |

## ドキュメント

- [アーキテクチャ概要](docs/architecture/overview.md)
- [マイクロサービス詳細設計](docs/architecture/microservices.md)
- [API 設計](docs/architecture/api-design.md)
- [データモデル](docs/architecture/data-model.md)
- [ディレクトリ構成](docs/architecture/directory-structure.md)
- [AWS サービス構成](docs/aws/services.md)
- [Kubernetes アーキテクチャ](docs/kubernetes/architecture.md)
- [Terraform 構成](docs/terraform/structure.md)
