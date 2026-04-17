# Go Microservices Chat

リアルタイムチャットプラットフォームを **Go + gRPC + Docker Compose** で構築するマイクロサービス学習プロジェクト。

## プロジェクトの目的

1. **マイクロサービスアーキテクチャを学ぶ** — モノレポ構成でのサービス分割、サービス間通信 (gRPC / WebSocket / Pub/Sub)、疎結合、API Gateway の役割を、実際に動くチャットアプリケーションを通じて理解する
2. **chat サービスを通じて Go の書き方・概念・フローを学ぶ** — 層構造 (Handler / Service / Repository)、依存性注入、`context.Context`、goroutine/channel、interface 設計など、Go バックエンドのイディオムを手を動かしながら習得する

**制約**: クラウド費用をかけずローカル (Docker Compose) で完結する。AWS/Kubernetes/Terraform は使わない。

## 想定規模

| 項目 | 想定値 |
|------|--------|
| 同時接続ユーザー数 | 〜100 人 |
| 総ユーザー数 | 〜1,000 人 |
| チャットルーム数 | 〜100 |
| メッセージ量 | 〜1,000 件/日 |

> 学習プロジェクトのため実トラフィックはない。ローカル Docker Compose で動く規模を想定する。

## 主な設計判断

| 判断 | 選定 | 理由 |
|------|------|------|
| サービス間通信 | **gRPC** (REST ではなく) | 型安全、コード生成、Streaming 対応。マイクロサービス間は REST より効率的 |
| クライアント向け API | **REST** (gRPC ではなく) | ブラウザからの利用が前提。gRPC はブラウザ互換性に制約がある |
| リアルタイム配信（サービス間） | **gRPC Server Streaming** (Redis Pub/Sub 統一ではなく) | chat-service が Redis に依存するのを避け、サービス間の疎結合を維持する（データストアを他サービスから直接触らないマイクロサービスの原則） |
| リアルタイム配信（クライアント向け） | **WebSocket** (SSE ではなく) | 双方向通信が必要（メッセージ送信 + 受信）。SSE はサーバー→クライアントの片方向のみ |
| 配信責務の分離 | **Redis Pub/Sub** | realtime-service 内で「受信」と「配信」を分離。1 インスタンス構成でも Pub/Sub を経由させ、N インスタンスに拡張しても同じコードで動く設計にする。Kafka は本規模では過剰 |
| データストア | **PostgreSQL** 統一 (DynamoDB ではなく) | 学習の焦点をマイクロサービス設計に絞る。NoSQL は対象外 |
| 認証 | **自前 JWT + bcrypt** (Cognito ではなく) | JWT の仕組み・トークン設計・パスワードハッシュを自分で実装して理解する |
| Proto 管理 | **Buf CLI** (protoc ではなく) | lint/generate/依存管理を統一的に扱える。protoc の複雑なプラグイン管理が不要 |
| DB ドライバー | **pgx v5** (database/sql ではなく) | PostgreSQL ネイティブ。接続プーリング (pgxpool) が標準搭載 |
| HTTP ルーター | **Chi v5** (Gin, Echo ではなく) | net/http 互換。標準ライブラリに近い設計で Go の慣習に沿う |
| ログ | **log/slog** (zap, zerolog ではなく) | Go 1.21 から標準ライブラリに含まれる。外部依存なしで構造化ログが書ける |
| 実行環境 | **Docker Compose** (Kubernetes ではなく) | ローカル完結、無料、再現性高い。本プロジェクトは K8s を対象外とする |

## アーキテクチャ

```mermaid
graph TD
    Client["クライアント"] -->|HTTPS| GW["API Gateway<br/>:8080"]
    Client -->|WSS| GW

    GW -->|gRPC| US["User Service<br/>:50051 / :8001"]
    GW -->|gRPC| CS["Chat Service<br/>:50052"]
    GW -->|WebSocket| RS["Realtime Service<br/>:8081"]

    RS -->|gRPC Server Streaming| CS
    RS -->|gRPC Unary| CS

    US --> PG1[("PostgreSQL<br/>userdb")]
    CS --> PG2[("PostgreSQL<br/>chatdb")]
    RS --> Redis[("Redis<br/>Pub/Sub")]
```

## サービス一覧

| サービス | 役割 | プロトコル | データストア |
|---------|------|-----------|------------|
| **user-service** | ユーザー管理・フレンド機能・認証 | REST + gRPC | PostgreSQL |
| **chat-service** | チャットルーム・メッセージ管理 | gRPC | PostgreSQL |
| **realtime-service** | WebSocket 接続・リアルタイム配信 | WebSocket + gRPC Server Streaming | Redis |
| **api-gateway** | JWT 検証・ルーティング・REST→gRPC 変換 | REST → gRPC 変換 | - |

## 技術スタック

| カテゴリ | 技術 |
|---------|------|
| 言語 | Go 1.22 |
| HTTP ルーター | Chi v5 |
| RPC | gRPC + Protocol Buffers (Buf CLI) |
| DB ドライバー | pgx v5 (PostgreSQL) |
| ログ | log/slog |
| コンテナ | Docker / Docker Compose |
| 認証 | 自前 JWT (golang-jwt/jwt) + bcrypt |
| キャッシュ/Pub/Sub | Redis (go-redis) |
| WebSocket | gorilla/websocket |

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
├── docker-compose.yml  # ローカル開発用 (PostgreSQL / Redis)
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
# user-service のテスト (DB 不要 - unit テスト)
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
| 2 | 認証・認可 (自前 JWT + bcrypt) | - |
| 3 | gRPC + マルチサービス + API Gateway | Proto 定義のみ完了 |
| 4 | リアルタイム通信 (WebSocket + gRPC Server Streaming + Redis Pub/Sub) | - |

> Phase 3 の Proto 定義 (`proto/user/v1/user.proto`) は先行して作成済み。Phase 2 の認証実装を終えてから Phase 3 で gRPC サーバー/クライアントを実装する。

## ドキュメント

- [マイクロサービス詳細設計](docs/architecture/microservices.md)
- [API 設計](docs/architecture/api-design.md)
- [データモデル](docs/architecture/data-model.md)
- [ディレクトリ構成](docs/architecture/directory-structure.md)
