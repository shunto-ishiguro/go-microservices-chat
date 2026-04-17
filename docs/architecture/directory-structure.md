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
│       └── ci.yml                   # CI: lint + test + buf lint
│
├── docs/                            # 設計ドキュメント
│   ├── architecture/
│   └── learning/
│
├── proto/                           # Protocol Buffers 定義（共有）
│   ├── buf.yaml                     # Buf 設定
│   ├── buf.gen.yaml                 # コード生成設定
│   ├── user/
│   │   └── v1/
│   │       └── user.proto           # (Phase 3 で使用)
│   ├── chat/
│   │   └── v1/
│   │       └── chat.proto           # (Phase 3 で追加)
│   └── realtime/
│       └── v1/
│           └── realtime.proto       # (Phase 4 で追加)
│
├── gen/                             # proto 生成コード (Go モジュール)
│   ├── go.mod
│   └── go/
│       ├── user/v1/
│       ├── chat/v1/
│       └── realtime/v1/
│
├── pkg/                             # 共有パッケージ
│   ├── go.mod
│   ├── go.sum
│   ├── auth/                        # (Phase 2) JWT 発行・検証
│   │   ├── claims.go
│   │   ├── issuer.go
│   │   └── verifier.go
│   ├── logger/                      # 構造化ログ (slog)
│   ├── middleware/                  # 共通ミドルウェア
│   │   ├── auth.go                  # JWT 検証 (Phase 2)
│   │   ├── logging.go               # リクエストログ
│   │   ├── recovery.go              # パニックリカバリ
│   │   └── cors.go                  # CORS 設定
│   ├── config/                      # 設定管理 (環境変数)
│   ├── errors/                      # 共通エラー型
│   ├── pagination/                  # ページネーション (cursor)
│   └── testutil/                    # テストユーティリティ
│
├── services/                        # マイクロサービス群
│   ├── user-service/                # (Phase 1 完了, Phase 2 で認証追加)
│   ├── chat-service/                # (Phase 3 で追加)
│   ├── realtime-service/            # (Phase 4 で追加)
│   └── api-gateway/                 # (Phase 3 で追加)
│
├── scripts/                         # ユーティリティスクリプト
│   ├── setup-local.sh               # ローカル環境セットアップ
│   ├── migrate.sh                   # DB マイグレーション
│   └── generate-proto.sh            # Proto コード生成
│
└── docker-compose.yml               # ローカル開発用 (PostgreSQL, Redis)
```

## サービスの内部構成

各マイクロサービスは以下の統一的な構成に従う (Phase 1 の user-service が基準)。

```
services/user-service/
├── go.mod                           # サービス固有の依存管理
├── go.sum
├── Dockerfile                       # マルチステージビルド (任意、本プロジェクトでは docker compose から build)
├── cmd/
│   └── server/
│       └── main.go                  # エントリーポイント (DI とサーバー起動)
├── internal/                        # 非公開パッケージ（外部インポート不可）
│   ├── config/
│   │   └── config.go                # 環境変数・設定読み込み
│   ├── domain/                      # ドメインモデル (エンティティ + 振る舞い)
│   │   ├── user.go
│   │   └── friendship.go
│   ├── repository/                  # データアクセス層
│   │   ├── repository.go            # Repository インターフェース定義
│   │   ├── postgres_user.go         # PostgreSQL 実装
│   │   └── postgres_user_test.go    # 統合テスト (docker 上の PG)
│   ├── service/                     # ビジネスロジック層
│   │   ├── user_service.go
│   │   └── user_service_test.go     # 単体テスト (fake repo)
│   └── handler/                     # トランスポート層
│       ├── rest/                    # REST ハンドラー (Phase 1+)
│       │   ├── handler.go
│       │   ├── user_handler.go
│       │   └── user_handler_test.go
│       └── grpc/                    # gRPC ハンドラー (Phase 3+)
│           ├── server.go
│           └── user_server.go
└── migrations/                      # DB マイグレーション (golang-migrate)
    ├── 001_create_users.up.sql
    ├── 001_create_users.down.sql
    └── 002_add_password_hash.up.sql # Phase 2 で追加
```

## レイヤードアーキテクチャ

各サービスは以下の 3 層で構成される。**上の層は下の層の interface に依存する** (依存関係逆転)。これにより、各層を独立してテストできる。

```
┌─────────────────────────────────┐
│          Handler 層              │  ← HTTP/gRPC リクエスト処理
│   (REST handler, gRPC server)    │     リクエストの変換・バリデーション
├─────────────────────────────────┤
│          Service 層              │  ← ビジネスロジック
│   (ユースケース・ドメインルール)    │     トランスポートに非依存
├─────────────────────────────────┤
│        Repository 層             │  ← データアクセス
│   (PostgreSQL, Redis)            │     interface で抽象化
└─────────────────────────────────┘
```

**依存の方向**: Handler → Service → Repository (interface)

### 各層の責務まとめ

| 層 | 依存先 | テスト方法 |
|----|--------|-----------|
| Handler | Service (interface) | `httptest` / `bufconn` + fake service |
| Service | Repository (interface) | fake repository (in-memory) |
| Repository | *pgxpool.Pool / redis.Client | 本物の DB (docker-compose で起動) |

### 依存性注入 (DI)

`cmd/server/main.go` で全ての依存を組み立てる。

```go
func main() {
    cfg := config.Load()

    pool, err := pgxpool.New(ctx, cfg.DBURL)
    // ...

    // 下から上に組み立てる
    userRepo  := repository.NewPostgresUserRepository(pool)
    userSvc   := service.NewUserService(userRepo)
    userHdlr  := resthandler.NewUserHandler(userSvc)

    r := chi.NewRouter()
    userHdlr.Register(r)

    // ...
}
```

---

## go.work 設定

```go
go 1.22

use (
    ./gen
    ./pkg
    ./services/user-service
    ./services/chat-service       // Phase 3 で追加
    ./services/realtime-service   // Phase 4 で追加
    ./services/api-gateway        // Phase 3 で追加
)
```

---

## 関連ドキュメント

- [マイクロサービス詳細](./microservices.md)
- [API 設計](./api-design.md)
- [データモデル](./data-model.md)
