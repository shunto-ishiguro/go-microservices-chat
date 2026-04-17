# ディレクトリ構成

## 設計方針

- **サービス内部は垂直分割 (package-per-feature)**：`internal/<domain>/` にエンティティ・サービス・リポジトリ・gRPC サーバーを集約。`user.Service`, `user.Repository` のように Go 標準ライブラリ風に呼び出せる
- **K8s マニフェストは `deploy/` に集約**：サービスごとに分割、Gateway 系は別ディレクトリ
- **api-gateway の Go 実装は持たない**：Envoy Gateway (YAML) が担当するため

---

## プロジェクト全体構成

```
go-microservices-chat/
├── go.work                          # Go Workspace 定義
├── Makefile                         # kind / buf / deploy コマンド集約
├── README.md
├── .github/workflows/ci.yml         # CI: lint + test + buf lint
│
├── docs/
│   ├── architecture/
│   └── learning/
│
├── proto/                           # Protocol Buffers 定義 (API の一次ソース)
│   ├── buf.yaml
│   ├── buf.gen.yaml
│   ├── user/v1/user.proto           # (Phase 0 から先行あり)
│   ├── chat/v1/chat.proto           # (Phase 2 で追加)
│   └── realtime/v1/realtime.proto   # (Phase 3 で追加)
│
├── gen/go/                          # Buf 生成コード (Go モジュール)
│   ├── user/v1/
│   ├── chat/v1/
│   └── realtime/v1/
│
├── pkg/                             # 共有パッケージ
│   ├── auth/                        # JWT 発行 (Phase 1)・JWKS 提供
│   ├── logger/                      # slog 初期化
│   ├── interceptor/                 # gRPC インターセプター
│   ├── config/                      # 環境変数ヘルパー
│   ├── errors/                      # 共通エラー型
│   └── testutil/
│
├── services/                        # マイクロサービス本体 (Go コード)
│   ├── user-service/                # Phase 1 で実装
│   ├── chat-service/                # Phase 2 で追加
│   └── realtime-service/            # Phase 3 で追加
│   # api-gateway/ は存在しない (Envoy Gateway が YAML で担当)
│
├── deploy/                          # K8s マニフェスト・Envoy Gateway 設定 (全部 Phase 4 で作成)
│   ├── kind-config.yaml             # kind クラスタ構成
│   ├── gateway/                     # Gateway API リソース (Phase 4)
│   │   ├── gateway.yaml             # Gateway (リスナー定義)
│   │   ├── user-public.yaml         # GRPCRoute (Register/Login/Refresh/Health)
│   │   ├── user-protected.yaml      # GRPCRoute (保護 RPC)
│   │   ├── chat-protected.yaml      # GRPCRoute (chat-service)
│   │   ├── realtime-route.yaml      # HTTPRoute (WebSocket)
│   │   ├── jwt-auth.yaml            # SecurityPolicy (RS256 + JWKS)
│   │   ├── rate-limit.yaml          # BackendTrafficPolicy (Rate Limit)
│   │   └── http-transcoder.yaml     # gRPC-JSON Transcoder (REST 自動公開)
│   ├── services/
│   │   ├── user-service/
│   │   │   ├── deployment.yaml
│   │   │   ├── service.yaml
│   │   │   ├── secret.yaml
│   │   │   ├── networkpolicy.yaml
│   │   │   └── migration-job.yaml
│   │   ├── chat-service/
│   │   └── realtime-service/
│   ├── postgres/
│   │   ├── statefulset.yaml
│   │   ├── service.yaml
│   │   └── secret.yaml
│   └── redis/
│       ├── deployment.yaml
│       └── service.yaml
│
└── scripts/                         # ユーティリティ
    ├── load-migrations.sh           # SQL を ConfigMap にロード
    └── proto-descriptor.sh          # Transcoder 用の descriptor 生成
```

> `docker-compose.yml` は **使わない**。Phase 1〜3 は `go run` + `docker run postgres/redis` でローカル開発、Phase 4 で kind に引っ越す。

---

## サービスの内部構成 (垂直分割)

各マイクロサービスは以下の統一的な構成に従う。

```
services/user-service/
├── go.mod                           # サービス固有の依存
├── go.sum
├── Dockerfile                       # multi-stage build
├── cmd/
│   └── server/main.go               # DI 組み立て + gRPC サーバー起動
├── internal/
│   ├── config/
│   │   └── config.go                # 環境変数読み込み (K8s Secret 由来)
│   └── user/                        # ★ 垂直分割の本体
│       ├── user.go                  # エンティティ + ドメインエラー
│       ├── service.go               # ビジネスロジック
│       ├── repository.go            # Repository interface + PostgreSQL 実装
│       ├── grpc_server.go           # proto ↔ domain 変換 + RPC ハンドラ
│       ├── auth.go                  # bcrypt / JWT 発行 (Phase 1)
│       ├── friend.go                # フレンド管理 (Phase 1)
│       └── *_test.go
└── migrations/                      # SQL (K8s Job で実行)
    ├── 001_create_users.up.sql
    ├── 002_add_password_hash.up.sql
    ├── 003_create_refresh_tokens.up.sql
    └── 004_create_friendships.up.sql
```

### 垂直分割を採用する理由

| 観点 | 水平分割 (`handler/service/repository/`) | 垂直分割 (`user/`, `friend/`) |
|------|-----------------------------------------|-------------------------------|
| 1 機能の変更範囲 | 3 フォルダに散らばる | 1 パッケージに閉じる |
| 呼び出し側の読み心地 | `handler.UserHandler` (冗長) | `user.Server` (Go 標準ライブラリ風) |
| Go コミュニティの傾向 | レイヤードアーキテクチャ文脈で採用 | Ben Johnson "Standard Package Layout" で推奨 |
| このプロジェクトでの選択 | — | **採用** (Go 標準ライブラリのイディオムに合わせる) |

### Phase ごとのパッケージ展開

| Phase | 追加されるパッケージ・ファイル | 備考 |
|-------|----------------------|------|
| 0 | `go.work` / `proto/buf.yaml` / `Makefile` / `gen/go/` | 骨組みのみ。`services/` `deploy/` は空 |
| 1 | `services/user-service/internal/user/`, `pkg/auth/`, `pkg/interceptor/` | CRUD + 認証 + フレンドを垂直分割で集約 |
| 2 | `services/chat-service/internal/room/`, `internal/message/`, `proto/chat/v1/` | チャットドメイン |
| 3 | `services/realtime-service/internal/hub/`, `internal/ws/`, `internal/presence/` | WebSocket ハブ + 接続管理 |
| 4 | `deploy/` 配下の全 YAML | K8s マニフェスト + Envoy Gateway 設定 |

---

## 層構造は「パッケージ内」で維持する

垂直分割でも **責務の分離 (層)** は意識する。

```
┌─────────────────────────────────┐
│ grpc_server.go                   │  ← トランスポート層
│   - proto ↔ domain 変換          │     リクエスト検証 / gRPC エラーコード変換
├─────────────────────────────────┤
│ service.go                       │  ← ビジネスロジック層
│   - ドメインルール (重複禁止など) │     トランスポート非依存
├─────────────────────────────────┤
│ repository.go                    │  ← データアクセス層
│   - PostgreSQL 実装              │     interface で抽象化
├─────────────────────────────────┤
│ user.go                          │  ← ドメイン層
│   - エンティティ / エラー型      │     他のどの層にも依存しない
└─────────────────────────────────┘
```

**依存の方向**: `grpc_server → service → repository (interface) → domain`

同じパッケージ内なので **import は発生しない**。ファイルの役割分担として層を保つ。

---

## 依存性注入 (DI)

`cmd/server/main.go` で全ての依存を組み立てる。

```go
func main() {
    cfg := config.Load()
    pool, _ := pgxpool.New(ctx, cfg.DBURL)

    userRepo   := user.NewPostgresRepository(pool)
    userSvc    := user.NewService(userRepo)
    userServer := user.NewGRPCServer(userSvc)

    grpcSrv := grpc.NewServer(
        grpc.ChainUnaryInterceptor(
            interceptor.Logging(logger),
            interceptor.Recovery(),
            interceptor.TrustedUserID(),  // Phase 1 から使う (JWT 検証は Envoy)
        ),
    )
    userv1.RegisterUserServiceServer(grpcSrv, userServer)

    lis, _ := net.Listen("tcp", cfg.GRPCAddr)
    grpcSrv.Serve(lis)
}
```

---

## deploy/ の役割

すべての K8s リソースをここに集約する。環境 (dev/prod) を分けたい場合は Kustomize overlays で `deploy/overlays/dev/` 等を足す (本プロジェクトでは dev のみ)。

### なぜ Helm ではなく素の YAML か

- **学習向け**: 生の K8s リソースを読んで書ける方が身につく
- Envoy Gateway のインストールのみ Helm を使う (公式チャート経由)
- Phase が進んだら Helm chart 化する発展課題はあり

---

## go.work 設定

```go
go 1.22

use (
    ./gen/go
    ./pkg
    ./services/user-service       // Phase 1 で追加
    ./services/chat-service        // Phase 2 で追加
    ./services/realtime-service    // Phase 3 で追加
)
```

---

## 関連ドキュメント

- [マイクロサービス詳細](./microservices.md)
- [API 設計](./api-design.md)
- [データモデル](./data-model.md)
