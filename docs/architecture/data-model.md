# データモデル設計

## 概要

本プロジェクトは **PostgreSQL に統一** する (学習の焦点をマイクロサービス設計に絞るため、NoSQL は対象外)。Redis は Pub/Sub (リアルタイム配信) と レートリミット用途に限定する。

各サービスは自分の DB のみを所有し、他サービスの DB には直接アクセスしない。

| サービス | DB | 主なテーブル |
|---------|-----|------------|
| user-service | userdb | `users`, `refresh_tokens` |
| chat-service | chatdb | `rooms`, `room_members`, `messages` |
| realtime-service | (Redis のみ) | Pub/Sub (ルームイベント配信) |

---

## user-service: userdb

### `users` テーブル

```sql
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           VARCHAR(255) UNIQUE NOT NULL,
    username        VARCHAR(50) UNIQUE NOT NULL,
    password_hash   VARCHAR(255) NOT NULL,  -- bcrypt (Phase 1 で追加)
    display_name    VARCHAR(100) NOT NULL,
    avatar_url      VARCHAR(500),
    status_text     VARCHAR(200),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_username ON users(username);
```

### `refresh_tokens` テーブル (Phase 1)

```sql
CREATE TABLE refresh_tokens (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash      VARCHAR(255) NOT NULL,  -- sha256 (検索用、復号不要)
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,
    user_agent      VARCHAR(255),
    ip_address      INET,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_hash ON refresh_tokens(token_hash);
```

---

## chat-service: chatdb

### `rooms` テーブル

全ルームは public。誰でも検索・参加できる。

```sql
CREATE TABLE rooms (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(100) NOT NULL,
    created_by      UUID NOT NULL,  -- user-service 所有、外部キーなし
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_rooms_name ON rooms(name);  -- 検索用
```

### `room_members` テーブル

誰がどのルームに参加しているかの記録。self-service で追加・削除する。`role` は持たない (全メンバーフラット)。

```sql
CREATE TABLE room_members (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id         UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL,
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(room_id, user_id)
);

CREATE INDEX idx_room_members_user ON room_members(user_id);
CREATE INDEX idx_room_members_room ON room_members(room_id);
```

> **注意**: `user_id` は外部キー参照にしていない (user-service 所有のため、境界を跨がない)。参照整合性は API 経由で担保する。

### `messages` テーブル

```sql
CREATE TABLE messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id         UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    sender_id       UUID NOT NULL,  -- user-service 所有、外部キーなし
    content         TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- (room_id, created_at, id) のタプルで cursor pagination を高速化。
-- ListByRoom が ORDER BY created_at DESC, id DESC + WHERE (created_at, id) < cursor で引くので、
-- room_id で絞り込んだ後は created_at + id でインデックススキャンが効く。同一秒の取りこぼし防止に id を含める。
CREATE INDEX idx_messages_room_created ON messages(room_id, created_at DESC, id DESC);
```

> **Phase 2 のスコープに絞った最小カラム**: テキストのみ、スレッド返信なし、添付なし。
> 将来 image / file / スレッドを追加する場合は `message_type` (enum) / `media_url` (TEXT) / `parent_id` (UUID, 自己参照 FK) を ALTER で追加できる。既存行は default で埋まるので破壊的変更にはならない。
> 「特定ユーザーの全発言」を引く API は無いので `sender_id` 単独 INDEX は作らない (INSERT コストの分損)。

---

## realtime-service: Redis

永続化は不要 (揮発データ)。realtime-service は Phase 2 の実装時点から Redis Pub/Sub を経由させて配信する (複数プロセス / 複数レプリカでも Go コード変更なし)。

```
# Pub/Sub チャンネル (PUBLISH した内容を全 realtime-service インスタンスが SUBSCRIBE)
room:<room_id>                  → ルーム内メッセージ配信用
```

> レートリミットや認証系の付加機能は infra 側 Envoyの責務。このリポジトリでは Redis を Pub/Sub 配信バス専用として扱う。

---

## Repository パターン

データアクセスはすべて interface 経由にして、テストでの差し替えを容易にする。Phase 1 ですでに user-service で実装済みのパターンを全サービスに適用する。

```go
// domain / interface は service 層が依存する
type UserRepository interface {
    Create(ctx context.Context, u *domain.User) error
    GetByID(ctx context.Context, id string) (*domain.User, error)
    GetByEmail(ctx context.Context, email string) (*domain.User, error)
    Update(ctx context.Context, u *domain.User) error
}

// Postgres 実装は infrastructure 層で提供
type postgresUserRepository struct {
    pool *pgxpool.Pool
}
```

単体テストでは in-memory fake で `go test ./...` を外部依存ゼロで PASS させる。実 PostgreSQL との結合検証は infra リポジトリ側で compose / K8s を立てて行う。

---

## 関連ドキュメント

- [マイクロサービス詳細](./microservices.md)
- [API 設計](./api-design.md)
