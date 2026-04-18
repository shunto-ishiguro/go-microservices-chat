# Phase 1: user-service (Go で完結)

---

## 学習目標

**K8s や Envoy には触れず**、Go のコードと `go run` + `docker run postgres` だけで user-service を完成させる。gRPC サーバー、PostgreSQL 接続、認証プリミティブ (bcrypt + RS256 JWT + JWKS)、プロフィール管理までを **すべてローカルで動作確認** する。

> user-service の責務は **ユーザー保管庫 + 認証発行所** のみ。friends / 1:1 DM / ユーザー単体検索はスコープ外 (「好きな公開ルームに参加して喋る」モデルのため)。

インフラ (K8s・Envoy・SecurityPolicy) は Phase 4 でまとめて扱う。そのため Phase 1 では **Go コードだけに集中** する。

| # | 目標 | 詳細 |
|---|------|------|
| 1 | Go の基本文法を理解する | 型・構造体・インターフェース・エラーハンドリング |
| 2 | gRPC サーバーを Go で書ける | Unary RPC・インターセプター |
| 3 | PostgreSQL に接続できる | `pgx` + マイグレーション |
| 4 | 認証プリミティブを自前実装できる | bcrypt / RS256 JWT Issuer / JWKS Handler |
| 5 | `TrustedUserID` Interceptor で信頼境界を Go 側で受け持てる | `x-user-id` メタデータを Context に |
| 6 | `bufconn` で gRPC の結合テストが書ける | K8s 不要、プロセス内 gRPC |

---

## 前提知識

- **Phase 0 完了**: `go.work` / `buf generate` / `Makefile` の骨格が整っていること
- プログラミング基礎
- SQL の基礎
- 公開鍵暗号と共通鍵暗号の違い（RS256 と JWKS の理解）

---

## 設計原則 (Phase 1 の間の大前提)

### サービスは `x-user-id` を **信頼するだけ**

Phase 1 では **JWT 検証を Go に書かない**。代わりに `TrustedUserID` Interceptor を用意し、`metadata.x-user-id` を Context に詰めるだけ。

```
Phase 1-3 (開発中):
  テストコード or grpcurl が metadata.x-user-id を直接注入
        ↓
  user-service: x-user-id を読んで Context に詰める
        ↓
  保護 RPC は Context の UserID で認可判定

Phase 4 (K8s 運用):
  Envoy Gateway が JWT 検証 → x-user-id を自動付与
        ↓  (サービス側のコードは一切変わらない)
  user-service: そのまま動く
```

**Phase 4 でサービスに手を入れないことが前提の設計**。後から「検証を Envoy に移管するリファクタ」は発生しない。

### JWT 発行は Phase 1 で実装する

`Login` / `Register` / `Refresh` RPC は Phase 1 でフル実装。Phase 1-3 の間は発行したトークンを検証する主体がいない (Envoy がいないため) が、**Phase 4 で Envoy を被せた瞬間から機能し始める** 前提。

---

## ステップ構成

| 部 | テーマ | ステップ |
|----|--------|----------|
| A | Go の足場作り | 1〜2 |
| B | 認証プリミティブ (bcrypt / RS256 / JWKS) | 3〜4 |
| C | PostgreSQL 接続 + Repository | 5〜6 |
| D | RPC 実装 (公開・保護) + Interceptor | 7〜10 |
| E | テスト・ログ | 11〜12 |

---

## A. Go の足場作り

### ステップ 1: Go 開発環境

- [ ] VS Code + Go 拡張機能
- [ ] `services/user-service/` を `go mod init`
- [ ] `go.work` に `./services/user-service` を追加

**確認ポイント**: `go build ./services/user-service/...` が通る。

---

### ステップ 2: Go 基本文法

- [ ] 型 / 構造体 / メソッド / インターフェース
- [ ] エラーハンドリング (`error`, `%w`, `errors.Is/As`)
- [ ] ポインタ・`context.Context`
- [ ] スライス・マップ
- [ ] goroutine / channel の概念 (Phase 3 で本格活用)

**確認ポイント**: 構造体にメソッドを定義し、interface を満たす実装ができる。

---

## B. 認証プリミティブ

### ステップ 3: bcrypt + RS256 Issuer

- [ ] RSA 鍵ペアを `openssl genrsa -out private.pem 2048` で生成
- [ ] `pkg/auth/issuer.go` に Issuer を実装

```go
type Issuer struct {
    privateKey *rsa.PrivateKey
    keyID      string
}

func (i *Issuer) IssueAccessToken(userID, username string) (string, error) {
    token := jwt.NewWithClaims(jwt.SigningMethodRS256, Claims{
        UserID:   userID,
        Username: username,
        RegisteredClaims: jwt.RegisteredClaims{
            Issuer:    "chat-app",
            Subject:   userID,
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
        },
    })
    token.Header["kid"] = i.keyID
    return token.SignedString(i.privateKey)
}
```

- [ ] ユニットテスト (`go test ./pkg/auth/...`)

**確認ポイント**: 発行されたトークンを [jwt.io](https://jwt.io/) で貼って RS256 として認識される。

---

### ステップ 4: JWKS Handler

Phase 4 で Envoy SecurityPolicy が JWKS を取得するが、**Phase 1 の段階で Handler は作り切る**。

- [ ] `pkg/auth/jwks.go` に JWKS 返却 HTTP Handler
- [ ] `kty: RSA`, `alg: RS256`, `use: sig` の形式
- [ ] ユニットテスト

```go
func (h *JWKSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    jwks := map[string]any{
        "keys": []map[string]any{
            {
                "kty": "RSA",
                "kid": h.keyID,
                "alg": "RS256",
                "use": "sig",
                "n":   base64url(h.publicKey.N.Bytes()),
                "e":   "AQAB",
            },
        },
    }
    json.NewEncoder(w).Encode(jwks)
}
```

**確認ポイント**: `go test ./pkg/auth/...` が PASS。`curl localhost:8082/.well-known/jwks.json` のスタブで JSON が返る。

---

## C. PostgreSQL 接続 + Repository

### ステップ 5: PostgreSQL をローカルで起動

**K8s は使わず、Docker で単発起動** する。docker-compose ではなく `docker run`。

```bash
docker run -d --name chat-postgres \
  -e POSTGRES_USER=chat \
  -e POSTGRES_PASSWORD=chat \
  -e POSTGRES_DB=userdb \
  -p 5432:5432 \
  postgres:15-alpine
```

- [ ] `pgx` v5 + `pgxpool` の接続コード (`internal/user/repository.go`)
- [ ] 接続文字列は環境変数 (`DATABASE_URL`)
- [ ] マイグレーションツール (golang-migrate) の導入
- [ ] マイグレーション SQL を `migrations/` に書く

```
services/user-service/migrations/
├── 001_create_users.up.sql / down.sql
├── 002_add_password_hash.up.sql / down.sql
└── 003_create_refresh_tokens.up.sql / down.sql
```

```bash
migrate -path services/user-service/migrations \
  -database "postgres://chat:chat@localhost:5432/userdb?sslmode=disable" up
```

**確認ポイント**: `psql postgres://chat:chat@localhost:5432/userdb` で接続でき、`\dt` で `users` と `refresh_tokens` の 2 テーブル。

---

### ステップ 6: Repository パターン実装

垂直分割のため、1 つのパッケージ内に interface と実装を共存。

```
services/user-service/internal/user/
├── user.go                  # エンティティ + ドメインエラー
├── repository.go            # interface + PostgreSQL 実装
├── repository_inmem.go      # インメモリ実装 (テスト用)
└── repository_test.go
```

```go
package user

type Repository interface {
    Create(ctx context.Context, u *User) error
    GetByID(ctx context.Context, id string) (*User, error)
    List(ctx context.Context, limit, offset int) ([]*User, error)
    Update(ctx context.Context, u *User) error
    Delete(ctx context.Context, id string) error
}
```

**確認ポイント**: PostgreSQL 実装とインメモリ実装の両方がテストで動く。

---

## D. RPC 実装 + Interceptor

### ステップ 7: 公開 RPC (Register / Login / Refresh)

- [ ] `internal/user/auth.go` に Register / Login / Refresh のビジネスロジック
- [ ] `Register`: bcrypt → `Repository.Create`
- [ ] `Login`: `GetByEmail` → bcrypt 検証 → Issuer で access/refresh 発行 + `refresh_tokens` INSERT
- [ ] `Refresh`: refresh 検証 → ローテーション
- [ ] gRPC ハンドラ (`internal/user/grpc_server.go`)

**確認ポイント**: bufconn のテストで Register → Login → Refresh が通る。

---

### ステップ 8: TrustedUserID Interceptor

**JWT 検証はしない**。`x-user-id` メタデータを Context に詰めるだけ。Phase 4 で Envoy がこの metadata を供給する。

```go
// pkg/interceptor/trusted_user_id.go
func TrustedUserID() grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
        md, ok := metadata.FromIncomingContext(ctx)
        if ok {
            if ids := md.Get("x-user-id"); len(ids) > 0 {
                ctx = context.WithValue(ctx, userIDKey, ids[0])
            }
        }
        return handler(ctx, req)
    }
}

func UserIDFromContext(ctx context.Context) (string, bool) {
    id, ok := ctx.Value(userIDKey).(string)
    return id, ok
}
```

**確認ポイント**: Interceptor のユニットテストが PASS。Phase 1 の段階では手動で metadata に `x-user-id` を載せてテスト。

---

### ステップ 9: 保護 RPC (GetUser / UpdateUser)

**Context から `UserID` を取り出す前提で書く**。認可チェックもここで。

- [ ] `GetUser` / `UpdateUser` をサービス層に追加
- [ ] gRPC ハンドラ実装
- [ ] リソース所有者認可: 他人のプロフィールは更新できない

```go
func (s *Service) UpdateUser(ctx context.Context, targetID string, ...) error {
    requesterID, _ := interceptor.UserIDFromContext(ctx)
    if requesterID != targetID {
        return status.Error(codes.PermissionDenied, "cannot update other user's profile")
    }
    // ...
}
```

**確認ポイント**: bufconn で metadata に x-user-id を注入したテストが動く。他人更新で `PermissionDenied`。

---

### ステップ 10: gRPC サーバー起動 (main.go)

- [ ] `cmd/server/main.go` で依存を組み立て
- [ ] gRPC サーバー (:50051) + JWKS HTTP サーバー (:8082) を goroutine で同時起動
- [ ] `TrustedUserID` Interceptor をチェーン

```go
func main() {
    cfg := config.Load()
    pool, _ := pgxpool.New(ctx, cfg.DatabaseURL)

    userRepo   := user.NewPostgresRepository(pool)
    issuer     := auth.NewIssuer(cfg.PrivateKey, cfg.KeyID)
    userSvc    := user.NewService(userRepo, issuer)
    userServer := user.NewGRPCServer(userSvc)

    grpcSrv := grpc.NewServer(grpc.ChainUnaryInterceptor(
        interceptor.Logging(logger),
        interceptor.TrustedUserID(),
    ))
    userv1.RegisterUserServiceServer(grpcSrv, userServer)
    healthpb.RegisterHealthServer(grpcSrv, health.NewServer())

    // gRPC
    go func() {
        lis, _ := net.Listen("tcp", ":50051")
        grpcSrv.Serve(lis)
    }()

    // JWKS HTTP
    http.Handle("/.well-known/jwks.json", auth.NewJWKSHandler(cfg.PublicKey, cfg.KeyID))
    http.ListenAndServe(":8082", nil)
}
```

**確認ポイント**:
```bash
# 起動
go run ./services/user-service/cmd/server

# 別ターミナルで動作確認
# 公開 RPC (x-user-id 不要)
grpcurl -plaintext -d '{"email":"alice@example.com","password":"password123",...}' \
  localhost:50051 user.v1.UserService/Register

grpcurl -plaintext -d '{"email":"alice@example.com","password":"password123"}' \
  localhost:50051 user.v1.UserService/Login

# 保護 RPC (x-user-id を手動で注入)
grpcurl -plaintext -H "x-user-id: <alice-uuid>" \
  -d '{"user_id":"<alice-uuid>"}' \
  localhost:50051 user.v1.UserService/GetUser

# JWKS
curl http://localhost:8082/.well-known/jwks.json

# Health Check
grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check
```

> **Phase 1 では JWT を発行するが、クライアントは検証しない**。代わりに `x-user-id` を直接注入して RPC を叩く。Phase 4 で Envoy が被さった瞬間、自動で JWT 検証されるようになる。

---

## E. テスト・ログ

### ステップ 11: bufconn による結合テスト

**K8s なしで gRPC をインプロセステスト**。Interceptor を含めて全体挙動を検証。

```go
lis := bufconn.Listen(1024 * 1024)
srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
    interceptor.TrustedUserID(),
))
userv1.RegisterUserServiceServer(srv, userServer)
go srv.Serve(lis)

// クライアントは metadata に x-user-id を詰める
ctx := metadata.AppendToOutgoingContext(ctx, "x-user-id", "alice-uuid")
resp, err := client.GetUser(ctx, &userv1.GetUserRequest{UserId: "bob-uuid"})
```

- [ ] Register → Login → Refresh の公開フロー
- [ ] x-user-id 付きで GetUser / UpdateUser
- [ ] 他人の UpdateUser が `PermissionDenied`

**確認ポイント**: `go test ./...` が PASS。

---

### ステップ 12: ログとエラーハンドリング

- [ ] `log/slog` (JSON)
- [ ] gRPC Interceptor でリクエスト ID を Context + ログに注入
- [ ] ドメインエラー → gRPC status マッピング
- [ ] `errors.Is` / `errors.As` 判定
- [ ] Graceful Shutdown (`srv.GracefulStop()`, SIGTERM 捕捉)
- [ ] ログマスク (`authorization` メタデータを出力しない)

**確認ポイント**: JSON ログが出力され、エラー時に適切な gRPC ステータスが返る。

---

## 成果物

Phase 1 完了時に以下が動作していること (**すべてローカルで確認可能**):

- [ ] `go run ./services/user-service/cmd/server` で gRPC + JWKS サーバーが起動
- [ ] `docker run postgres` + `migrate ... up` で DB 初期化できる
- [ ] `grpcurl` で Register / Login / Refresh / GetUser / UpdateUser が動く (x-user-id は手動注入)
- [ ] `curl http://localhost:8082/.well-known/jwks.json` で JWKS が返る
- [ ] bcrypt でパスワード保管、RS256 で JWT 発行
- [ ] `TrustedUserID` Interceptor で Context に UserID
- [ ] リソース所有者認可 (`PermissionDenied`)
- [ ] `go test ./...` が PASS (bufconn 結合テスト含む)

> **まだ無いもの** (Phase 4 で追加): Envoy Gateway、SecurityPolicy、NetworkPolicy、Dockerfile、K8s マニフェスト、kind クラスタ。

### ディレクトリ構成 (Phase 1 完了時)

```
go-microservices-chat/
├── pkg/
│   ├── auth/                        # RS256 Issuer + JWKS Handler
│   └── interceptor/
│       └── trusted_user_id.go
├── services/user-service/
│   ├── cmd/server/main.go           # gRPC :50051 + JWKS HTTP :8082
│   ├── internal/
│   │   ├── config/
│   │   └── user/                    # 垂直分割: ドメイン一式
│   │       ├── user.go
│   │       ├── service.go
│   │       ├── repository.go
│   │       ├── repository_inmem.go
│   │       ├── grpc_server.go
│   │       ├── auth.go              # Register/Login/Refresh/Logout
│   │       └── *_test.go
│   ├── migrations/
│   └── go.mod
├── proto/                           # (Phase 0 から)
├── gen/go/                          # (Phase 0 から)
└── go.work
```

> `Dockerfile`・`deploy/` は Phase 4 で追加。

### ローカル起動フロー (Phase 1 完了時)

```mermaid
sequenceDiagram
    participant Dev as 開発者
    participant DB as docker run postgres
    participant US as go run user-service
    participant GC as grpcurl

    Dev->>DB: docker run postgres
    Dev->>DB: migrate up
    Dev->>US: go run ./services/user-service/cmd/server
    Note over US: gRPC :50051<br/>JWKS HTTP :8082

    Dev->>GC: grpcurl Register
    GC->>US: Register (公開 RPC)
    US->>DB: INSERT users
    US->>GC: ok

    Dev->>GC: grpcurl -H "x-user-id: <alice>" GetUser
    GC->>US: GetUser + metadata
    US->>US: TrustedUserID → Context
    US->>DB: SELECT users
    US->>GC: GetUserResponse
```

---

## 学べる技術

| カテゴリ | 技術 | 用途 |
|----------|------|------|
| 言語 | Go | メイン開発言語 |
| RPC | gRPC + Protocol Buffers | 型安全な通信 |
| コード生成 | Buf CLI | proto から Go コード生成 |
| データベース | PostgreSQL / pgx v5 | リレーショナル DB |
| マイグレーション | golang-migrate (CLI) | スキーマ管理 |
| テスト | testing / bufconn | K8s 不要でインプロセス結合テスト |
| ログ | log/slog | 構造化ロギング |
| パスワード | bcrypt | ソルト付きハッシュ |
| トークン | JWT (RS256) / golang-jwt | ステートレス認証 + JWKS で公開鍵配布 |
| 信頼境界設計 | TrustedUserID Interceptor | Phase 4 で Envoy と接合する前提のインタフェース |

---

## 次のフェーズ

Phase 1 が完了したら [Phase 2: chat-service 追加 + サービス間 gRPC 通信](./phase-2.md) に進む。user-service はそのまま動かしつつ、chat-service を別プロセスで起動し、2 プロセス間で gRPC 通信する体験をする。**K8s・Envoy は Phase 4 まで登場しない**。
