# Go Microservices Chat

リアルタイムチャットプラットフォームを **Go + AWS + Kubernetes** で構築するマイクロサービス学習プロジェクト。

## プロジェクトの目的

モノレポ構成のマイクロサービスアーキテクチャにおけるベストプラクティスを、実際に動くチャットアプリケーションを通じて学ぶ。

## 想定規模

| 項目 | 想定値 |
|------|--------|
| 同時接続ユーザー数 | 〜1,000 人 |
| 総ユーザー数 | 〜10,000 人 |
| チャットルーム数 | 〜500 |
| メッセージ量 | 〜10,000 件/日 |

> 学習プロジェクトのため実トラフィックはないが、上記規模を想定して設計・実装する。

## 主な設計判断

| 判断 | 選定 | 理由 |
|------|------|------|
| サービス間通信 | **gRPC** (REST ではなく) | 型安全、コード生成、Streaming 対応。マイクロサービス間は REST より効率的 |
| クライアント向け API | **REST** (gRPC ではなく) | ブラウザからの利用が前提。gRPC はブラウザ互換性に制約がある |
| リアルタイム配信（サービス間） | **gRPC Server Streaming** (Redis Pub/Sub 統一ではなく) | chat-service が Redis に依存するのを避け、サービス間の疎結合を維持する（データストアを他サービスから直接触らないマイクロサービスの原則） |
| リアルタイム配信（クライアント向け） | **WebSocket** (SSE ではなく) | 双方向通信が必要（メッセージ送信 + 受信）。SSE はサーバー→クライアントの片方向のみ |
| インスタンス間同期 | **Redis Pub/Sub** | realtime-service の複数インスタンス間でメッセージを同期。Kafka は本規模では過剰 |
| Proto 管理 | **Buf CLI** (protoc ではなく) | lint/generate/依存管理を統一的に扱える。protoc の複雑なプラグイン管理が不要 |
| DB ドライバー | **pgx v5** (database/sql ではなく) | PostgreSQL ネイティブ。接続プーリング (pgxpool) が標準搭載 |
| HTTP ルーター | **Chi v5** (Gin, Echo ではなく) | net/http 互換。標準ライブラリに近い設計で Go の慣習に沿う |
| ログ | **log/slog** (zap, zerolog ではなく) | Go 1.21 から標準ライブラリに含まれる。外部依存なしで構造化ログが書ける |
| コンテナオーケストレーション | **Kubernetes** (ECS ではなく) | サービスごとの独立デプロイ・スケーリング・ヘルスチェックをマニフェストで宣言的に管理。マイクロサービスの運用標準 |
| IaC | **Terraform** (CloudFormation ではなく) | AWS 以外にも対応可能。宣言的なインフラ管理のデファクトスタンダード |

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
| **realtime-service** | WebSocket 接続・リアルタイム配信 | WebSocket + gRPC Server Streaming | Redis |
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

- [マイクロサービス詳細設計](docs/architecture/microservices.md)
- [API 設計](docs/architecture/api-design.md)
- [データモデル](docs/architecture/data-model.md)
- [ディレクトリ構成](docs/architecture/directory-structure.md)
- [AWS サービス構成](docs/aws/services.md)
- [Kubernetes アーキテクチャ](docs/kubernetes/architecture.md)
- [Terraform 構成](docs/terraform/structure.md)
