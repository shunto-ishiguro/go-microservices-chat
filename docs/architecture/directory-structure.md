# ディレクトリ構成

## 設計方針

- **サービス内部は垂直分割 (package-per-feature)**: `internal/<domain>/` にエンティティ・サービス・リポジトリ・gRPC サーバーを集約。`user.Service`, `user.Repository` のように Go 標準ライブラリ風に呼び出せる
- **本番向け K8s マニフェストは別リポジトリ**: Deployment / Gateway API / SecurityPolicy / NetworkPolicy / Helm は infra 側。env var で全ての接続先を受け取る 12-factor 前提
- **dev / E2E 専用の `compose.yaml` + `envoy.yaml` は本リポジトリに持つ** (Phase 4): Envoy standalone 経由で JWT 検証経路まで含む E2E を手元で回すため
- **api-gateway の Go 実装は持たない**: JWT 検証は Envoy (dev: standalone / 本番: Envoy Gateway) の責務

---

## プロジェクト全体構成

```
go-microservices-chat/
├── go.work                          # Go Workspace 定義
├── Makefile                         # proto-gen / test / image-build
├── README.md
├── .gitignore
│
├── docs/
│   ├── architecture/
│   ├── flow/
│   └── phase/
│
├── proto/                           # Protocol Buffers 定義 (API の一次ソース)
│   ├── buf.yaml
│   ├── buf.gen.yaml
│   ├── user/v1/user.proto           # (Phase 1)
│   └── chat/v1/chat.proto           # (Phase 1: Room / Phase 2: Message 追加)
│   # realtime-service は WebSocket + Redis Pub/Sub のみで、gRPC を公開しない → realtime.proto なし
│
├── gen/go/                          # Buf 生成コード (Go モジュール)
│   ├── user/v1/
│   └── chat/v1/
│
├── pkg/                             # 共有パッケージ
│   ├── auth/                        # JWT 発行 (Issuer) / JWKS Handler / RequesterID
│   └── interceptor/                 # gRPC Logging Interceptor
│
├── services/                        # マイクロサービス本体 (Go コード)
│   ├── user-service/                # Phase 1 で実装 (+ Phase 3 で Dockerfile)
│   ├── chat-service/                # Phase 1: Room / Phase 2: Message (+ Phase 3 で Dockerfile)
│   └── realtime-service/            # Phase 2 で追加 (+ Phase 3 で Dockerfile)
│
├── compose.yaml                     # ★ Phase 4: dev / E2E 専用 (本番向けではない)
├── envoy.yaml                       # ★ Phase 4: Envoy standalone 設定 (JWT filter + routes)
└── scripts/
    └── e2e/                         # ★ Phase 4: E2E シナリオ (up / register-login / chat / auth-failures / down)
```

> **本番向け K8s マニフェスト (`deploy/`, Helm chart 等) は本リポジトリに存在しない**。infra リポジトリが責務を持つ。
>
> `compose.yaml` / `envoy.yaml` はあくまで **dev / E2E 専用の軽量 stack**。永続ボリュームや HA や Observability は持たない — それらは infra 側。

---

## サービスの内部構成 (垂直分割)

各マイクロサービスは以下の統一的な構成に従う。

```
services/user-service/
├── go.mod                           # サービス固有の依存
├── go.sum
├── Dockerfile                       # multi-stage build (Phase 3)
├── cmd/
│   └── server/main.go               # DI 組み立て + gRPC サーバー起動
├── internal/
│   ├── config/
│   │   └── config.go                # 環境変数読み込み
│   └── user/                        # ★ 垂直分割: ドメインに必要な全てが同居
│       ├── user.go                  # エンティティ + ドメインエラー
│       ├── service.go               # ビジネスロジック (bcrypt + pkg/auth.Issuer 呼び出しもここ)
│       ├── repository.go            # Repository interface + PostgreSQL 実装
│       ├── repository_inmem.go      # テスト用 InMem 実装
│       ├── grpc.go                  # GRPCAdapter (proto ↔ domain 変換 + RPC ハンドラ)
│       └── *_test.go
└── migrations/                      # SQL (infra 側の migration runner が実行)
    ├── 001_create_users.up.sql
    └── 002_create_refresh_tokens.up.sql
```

### 複数ドメインが同居するサービスのバリエーション (chat-service)

chat-service は proto 側で **1 つの gRPC サービス (`ChatService`) に Room と Message の RPC が両方含まれる**。
Go で生成される `ChatServiceServer` interface は 1 つなので、最終的に登録される struct も 1 つ必要。
**Phase 1 時点では Room のみ** なので `room.GRPCAdapter` が単独でその役を担う (Message 系 RPC は埋め込んだ `UnimplementedChatServiceServer` のデフォルト応答)。
**Phase 2 で Message が加わる**時に初めて `internal/grpc/` を新設し、両アダプタを named field で保持して各 RPC を forward する薄い合流 struct を置く (両ドメインの型 simple name が `GRPCAdapter` で衝突するため embed は使えない)。

```
services/chat-service/                # Phase 1 時点 (Room のみ)
├── cmd/server/main.go
├── internal/
│   ├── config/
│   ├── room/                        # Room 集約 (rooms / room_members)
│   │   ├── room.go
│   │   ├── service.go               # Create/Get/List/Search/Join/Leave/EnsureMember
│   │   ├── repository.go / repository_inmem.go
│   │   ├── grpc.go                  # GRPCAdapter (ChatServiceServer を単独で満たす)
│   │   └── *_test.go
│   └── userclient/                  # user-service 呼び出し (member enrich)
│       ├── client.go
│       └── fake.go                  # テスト用
└── migrations/

# Phase 2 以降の追加構造 (Message ドメインが加わる時)
├── internal/message/                # Message 集約
│   ├── message.go
│   ├── service.go
│   ├── grpc.go                      # GRPCAdapter (Message RPC のみ)
│   └── *_test.go
└── internal/grpc/                   # 薄い合流層
    └── server.go                    # rooms / messages を named field で保持 + RPC を forward
```

**ルール**:

- **grpc.go はドメインパッケージと同居させる**。repository.go と同じ扱いで、ドメインに関する全てが 1 パッケージに閉じる (垂直分割の純化)
- proto 型の import はドメインパッケージ内で許容する。**ただし service.go (業務ロジック) は proto に触れない** — proto は grpc.go 側に閉じ込める
- `UnimplementedXxxServer` の埋め込みは合流層 1 箇所だけに置く (forward-compat のため)。各ドメインアダプタには置かない
- 1 proto service / 1 ドメインなら grpc.go 単独 (user-service パターン)。複数ドメイン跨ぎなら薄い `internal/grpc/` 合流層 (chat-service Phase 2 パターン)
- 合流層では **アダプタを embed せず named field で保持する**。Go の埋め込みは型の simple name をフィールド名にするため、両ドメインの型 (`room.GRPCAdapter` / `message.GRPCAdapter`) を同時に embed すると名前衝突する。各 RPC は `func (s *Server) CreateRoom(...) { return s.rooms.CreateRoom(...) }` の形で明示的に forward する

### 垂直分割を採用する理由

| 観点 | 水平分割 (`handler/service/repository/`) | 垂直分割 (`user/`, `room/`) |
|------|-----------------------------------------|-------------------------------|
| 1 機能の変更範囲 | 3 フォルダに散らばる | 1 パッケージに閉じる |
| 呼び出し側の読み心地 | `handler.UserHandler` (冗長) | `user.GRPCAdapter` (Go 標準ライブラリ風) |
| Go コミュニティの傾向 | レイヤードアーキテクチャ文脈で採用 | Ben Johnson "Standard Package Layout" で推奨 |
| このプロジェクトでの選択 | — | **採用** (Go 標準ライブラリのイディオムに合わせる) |

### Phase ごとのパッケージ展開

| Phase | 追加されるパッケージ・ファイル | 備考 |
|-------|----------------------|------|
| 1 | `go.work` / `proto/` + `gen/go/` / `pkg/auth/` + `pkg/interceptor/` / `services/user-service/` / `services/chat-service/internal/{room,userclient}/` | user 機能 (JWT 発行 + JWKS 配信) + Room 機能を実装。**JWT 検証ロジックは書かない** |
| 2 | `services/chat-service/internal/message/` + `internal/grpc/` 合流層 / `services/realtime-service/` (hub / pubsub / chatclient / ws) | Message + realtime-service (WebSocket + Redis Pub/Sub)。最初から Pub/Sub 採用 |
| 3 | 3 サービスの `Dockerfile` | distroless + multi-stage build |
| 4 | `compose.yaml` / `envoy.yaml` / `scripts/e2e/*.sh` | dev / E2E 専用 stack。Envoy standalone 経由で JWT 検証経路まで含む全フローを確認 |

---

## 層構造は「パッケージ内」で維持する

垂直分割でも **責務の分離 (層)** は意識する。

```
┌─────────────────────────────────┐
│ grpc.go                          │  ← トランスポート層
│   - GRPCAdapter (proto ↔ domain 変換) │   リクエスト検証 / gRPC エラーコード変換
├─────────────────────────────────┤
│ service.go                       │  ← ビジネスロジック層
│   - ドメインルール (重複禁止など) │     トランスポート非依存 (proto を import しない)
├─────────────────────────────────┤
│ repository.go                    │  ← データアクセス層
│   - PostgreSQL 実装              │     interface で抽象化
├─────────────────────────────────┤
│ user.go / room.go                │  ← ドメイン層
│   - エンティティ / エラー型      │     他のどの層にも依存しない
└─────────────────────────────────┘
```

**依存の方向**: `grpc.go → service.go → repository.go (interface) → domain`

同じパッケージ内なので **import は発生しない**。ファイルの役割分担として層を保つ (= 垂直分割内での水平層構造)。

---

## 依存性注入 (DI)

`cmd/server/main.go` で全ての依存を組み立てる。

```go
// services/user-service/cmd/server/main.go (要点抜粋、エラー処理省略)
func run(logger *slog.Logger) error {
    cfg, _ := config.Load()
    priv, _ := jwt.ParseRSAPrivateKeyFromPEM([]byte(cfg.JWTPrivateKey))  // PEM → *rsa.PrivateKey
    pool, _ := pgxpool.New(ctx, cfg.DatabaseURL)

    // DI: 下から上に組み立てる。Repository は interface なのでテストでは InMem に差し替えられる。
    repo   := user.NewPostgresRepository(pool)
    issuer := auth.NewIssuer(priv, cfg.JWTKeyID)
    svc    := user.NewService(repo, issuer)

    grpcSrv := grpc.NewServer(
        grpc.ChainUnaryInterceptor(interceptor.Logging(logger)),
    )
    userv1.RegisterUserServiceServer(grpcSrv, user.NewGRPCAdapter(svc))
    // ハンドラ内では auth.RequesterID(ctx) で呼び出し元を取得 (Envoy が x-user-id を注入)

    // gRPC :50051 + JWKS HTTP :8082 を別 goroutine で同時起動 (実装は graceful shutdown 込み)
    lis, _ := net.Listen("tcp", cfg.GRPCAddr)
    return grpcSrv.Serve(lis)
}
```

> chat-service / realtime-service も同じ DI パターン。chat-service は Phase 2 で `room.GRPCAdapter` + `message.GRPCAdapter` を `internal/grpc.NewServer(...)` で束ねた合流層を `RegisterChatServiceServer` に渡す。realtime-service は gRPC サーバーを持たず HTTP `/ws` ハンドラ + Hub goroutine + Subscriber goroutine の構成。

---

## go.work 設定

```go
go 1.22

use (
    ./gen/go
    ./pkg
    ./services/user-service       // Phase 1 で追加
    ./services/chat-service        // Phase 1 で追加 (Room) / Phase 2 で Message 拡張
    ./services/realtime-service    // Phase 2 で追加
)
```

---

## infra リポジトリとの境界

このリポジトリが責任を持つもの:

- Go サービス実装
- proto 定義
- Dockerfile (Phase 3)
- **dev / E2E 用の `compose.yaml` + `envoy.yaml` + `scripts/e2e/*.sh`** (Phase 4)
- 単体テスト (外部依存ゼロで PASS) + E2E (手元で JWT 検証経路まで)

このリポジトリで **やらない** もの (infra repo の責務):

- 本番向け K8s manifest / Helm chart (Deployment / StatefulSet / Gateway API / SecurityPolicy / NetworkPolicy)
- TLS 証明書 / 本番 JWT 鍵の管理
- 永続ボリューム / HA / 自動 failover
- 本番 migration runner (compose では手動スクリプトで代用)
- Rate Limit / Observability / CI/CD pipeline

---

## 関連ドキュメント

- [マイクロサービス詳細](./microservices.md)
- [API 設計](./api-design.md)
- [データモデル](./data-model.md)
