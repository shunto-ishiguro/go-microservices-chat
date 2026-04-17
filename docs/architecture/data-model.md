# データモデル設計

## 概要

本プロジェクトは **PostgreSQL に統一** する (学習の焦点をマイクロサービス設計に絞るため、NoSQL は対象外)。Redis はキャッシュ・Pub/Sub・プレゼンスの用途に限定する。

各サービスは自分の DB のみを所有し、他サービスの DB には直接アクセスしない。

| サービス | DB | 主なテーブル |
|---------|-----|------------|
| user-service | userdb | `users`, `refresh_tokens`, `friendships` |
| chat-service | chatdb | `rooms`, `room_members`, `messages`, `read_receipts` |
| realtime-service | (Redis のみ) | プレゼンス、接続マッピング、Pub/Sub |

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
    is_online       BOOLEAN DEFAULT false,
    last_seen_at    TIMESTAMPTZ,
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

### `friendships` テーブル

```sql
CREATE TABLE friendships (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id),
    friend_id       UUID NOT NULL REFERENCES users(id),
    status          VARCHAR(20) NOT NULL DEFAULT 'pending',
                    -- pending, accepted, blocked
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, friend_id)
);

CREATE INDEX idx_friendships_user ON friendships(user_id, status);
CREATE INDEX idx_friendships_friend ON friendships(friend_id, status);
```

---

## chat-service: chatdb

### `rooms` テーブル

```sql
CREATE TABLE rooms (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(100),
    type            VARCHAR(20) NOT NULL DEFAULT 'direct',
                    -- direct (1:1), group
    created_by      UUID NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### `room_members` テーブル

```sql
CREATE TABLE room_members (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id         UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL,
    role            VARCHAR(20) NOT NULL DEFAULT 'member',
                    -- owner, admin, member
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
    message_type    VARCHAR(20) NOT NULL DEFAULT 'text',
                    -- text, image, file
    parent_id       UUID REFERENCES messages(id),  -- スレッド返信
    is_edited       BOOLEAN DEFAULT false,
    is_deleted      BOOLEAN DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_messages_room_created ON messages(room_id, created_at DESC);
CREATE INDEX idx_messages_sender ON messages(sender_id);
```

### `read_receipts` テーブル

```sql
CREATE TABLE read_receipts (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id                 UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id                 UUID NOT NULL,
    last_read_message_id    UUID REFERENCES messages(id),
    last_read_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(room_id, user_id)
);

CREATE INDEX idx_read_receipts_room ON read_receipts(room_id);
```

---

## realtime-service: Redis

永続化は不要 (揮発データ)。本プロジェクトの realtime-service は **1 インスタンス構成** だが、将来 N インスタンスに拡張しても同じコードで動くよう、メッセージ配信は Redis Pub/Sub を経由させる。

```
# プレゼンス情報
presence:<user_id>              → "online" | "offline"
                                  TTL: 60 秒 (ハートビートで更新)

# Pub/Sub チャンネル (自分 → Redis → 自分に戻ってきて Hub に流す)
channel:room:<room_id>          → ルーム内メッセージ配信用
channel:user:<user_id>          → 個人向け通知用

# レートリミット (Phase 4: Envoy BackendTrafficPolicy 経由)
ratelimit:login:<ip>            → カウンター (TTL つき)

# 将来 N インスタンス時の接続マッピング (1 インスタンスでは不要)
# ws:connections:<user_id>      → Set of <instance_id>:<connection_id>
```

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

単体テストでは in-memory fake、結合テストでは kind クラスタ上で起動した PostgreSQL (port-forward 経由) を使う。

---

## 関連ドキュメント

- [マイクロサービス詳細](./microservices.md)
- [API 設計](./api-design.md)
