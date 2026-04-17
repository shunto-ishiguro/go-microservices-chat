# データモデル設計

## 概要

本プロジェクトでは、学習の段階に応じてデータストアを進化させる。

- **Phase 1-3**: PostgreSQL（SQL の基礎を学ぶ）
- **Phase 4**: DynamoDB へ移行（NoSQL + AWS スキルを学ぶ）
- **全 Phase 共通**: Redis（キャッシュ・Pub/Sub）

## Phase 1-3: PostgreSQL スキーマ

### users DB（User Service 所有）

```sql
-- ユーザー基本情報
CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cognito_sub VARCHAR(255) UNIQUE,
    email       VARCHAR(255) UNIQUE NOT NULL,
    username    VARCHAR(50) UNIQUE NOT NULL,
    display_name VARCHAR(100) NOT NULL,
    avatar_url  VARCHAR(500),
    status_text VARCHAR(200),
    is_online   BOOLEAN DEFAULT false,
    last_seen_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_cognito_sub ON users(cognito_sub);

-- フレンドリレーション
CREATE TABLE friendships (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id),
    friend_id   UUID NOT NULL REFERENCES users(id),
    status      VARCHAR(20) NOT NULL DEFAULT 'pending',
                -- pending, accepted, blocked
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, friend_id)
);

CREATE INDEX idx_friendships_user ON friendships(user_id, status);
CREATE INDEX idx_friendships_friend ON friendships(friend_id, status);
```

### messages DB（Chat Service 所有）

```sql
-- チャットルーム
CREATE TABLE rooms (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(100),
    type        VARCHAR(20) NOT NULL DEFAULT 'direct',
                -- direct (1:1), group
    created_by  UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ルームメンバー
CREATE TABLE room_members (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id     UUID NOT NULL REFERENCES rooms(id),
    user_id     UUID NOT NULL,
    role        VARCHAR(20) NOT NULL DEFAULT 'member',
                -- owner, admin, member
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(room_id, user_id)
);

CREATE INDEX idx_room_members_user ON room_members(user_id);
CREATE INDEX idx_room_members_room ON room_members(room_id);

-- メッセージ
CREATE TABLE messages (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id     UUID NOT NULL REFERENCES rooms(id),
    sender_id   UUID NOT NULL,
    content     TEXT NOT NULL,
    message_type VARCHAR(20) NOT NULL DEFAULT 'text',
                -- text, image, file
    media_url   VARCHAR(500),
    parent_id   UUID REFERENCES messages(id),  -- スレッド返信
    is_edited   BOOLEAN DEFAULT false,
    is_deleted  BOOLEAN DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_messages_room_created ON messages(room_id, created_at DESC);
CREATE INDEX idx_messages_sender ON messages(sender_id);

-- 既読管理
CREATE TABLE read_receipts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id     UUID NOT NULL REFERENCES rooms(id),
    user_id     UUID NOT NULL,
    last_read_message_id UUID REFERENCES messages(id),
    last_read_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(room_id, user_id)
);

CREATE INDEX idx_read_receipts_room ON read_receipts(room_id);
```

## Phase 4: DynamoDB テーブル設計

### アクセスパターン分析

DynamoDB の設計はアクセスパターンから逆算する。

| # | アクセスパターン | 頻度 |
|---|----------------|------|
| 1 | ルーム ID でメッセージ一覧を取得（新しい順） | 非常に高い |
| 2 | ユーザー ID で参加ルーム一覧を取得 | 高い |
| 3 | ルーム ID でメンバー一覧を取得 | 高い |
| 4 | ユーザー ID で通知一覧を取得（新しい順） | 高い |
| 5 | 特定メッセージを ID で取得 | 中 |
| 6 | ユーザー ID + ルーム ID で既読状態を取得 | 高い |
| 7 | ルーム ID で最新メッセージ N 件を取得 | 高い |

### messages テーブル（Chat Service 所有）

```
テーブル名: chat-messages

Partition Key (PK): ROOM#<room_id>
Sort Key (SK):      MSG#<timestamp>#<message_id>

GSI1:
  PK: USER#<user_id>
  SK: ROOM#<room_id>#<timestamp>

属性:
  - pk          (S): "ROOM#<room_id>"
  - sk          (S): "MSG#<2024-01-15T10:30:00Z>#<msg_id>"
  - gsi1pk      (S): "USER#<user_id>"
  - gsi1sk      (S): "ROOM#<room_id>#<2024-01-15T10:30:00Z>"
  - sender_id   (S): "<user_id>"
  - content     (S): "メッセージ本文"
  - message_type(S): "text" | "image" | "file"
  - media_url   (S): "https://..."
  - parent_id   (S): "<message_id>"  (スレッド返信)
  - is_edited   (BOOL): false
  - created_at  (S): ISO 8601
  - updated_at  (S): ISO 8601
```

**クエリ例**:
```
# パターン1: ルームのメッセージ一覧（新しい順）
PK = "ROOM#abc123", SK begins_with "MSG#", ScanIndexForward=false

# パターン7: ルームの最新N件
PK = "ROOM#abc123", SK begins_with "MSG#", ScanIndexForward=false, Limit=N
```

### rooms テーブル（Chat Service 所有）

```
テーブル名: chat-rooms

Partition Key (PK): ROOM#<room_id>
Sort Key (SK):      METADATA | MEMBER#<user_id> | READ#<user_id>

GSI1 (ユーザーの参加ルーム検索):
  PK: USER#<user_id>
  SK: ROOM#<room_id>

アイテムタイプ 1 - ルームメタデータ:
  - pk   (S): "ROOM#<room_id>"
  - sk   (S): "METADATA"
  - name (S): "ルーム名"
  - type (S): "direct" | "group"
  - created_by (S): "<user_id>"
  - created_at (S): ISO 8601

アイテムタイプ 2 - ルームメンバー:
  - pk      (S): "ROOM#<room_id>"
  - sk      (S): "MEMBER#<user_id>"
  - gsi1pk  (S): "USER#<user_id>"
  - gsi1sk  (S): "ROOM#<room_id>"
  - role    (S): "owner" | "admin" | "member"
  - joined_at(S): ISO 8601

アイテムタイプ 3 - 既読状態:
  - pk               (S): "ROOM#<room_id>"
  - sk               (S): "READ#<user_id>"
  - last_read_msg_id (S): "<message_id>"
  - last_read_at     (S): ISO 8601
```

**クエリ例**:
```
# パターン2: ユーザーの参加ルーム一覧
GSI1: PK = "USER#user123", SK begins_with "ROOM#"

# パターン3: ルームのメンバー一覧
PK = "ROOM#abc123", SK begins_with "MEMBER#"

# パターン6: 既読状態の取得
PK = "ROOM#abc123", SK = "READ#user123"
```

### notifications テーブル（Notification Service 所有）

```
テーブル名: notifications

Partition Key (PK): USER#<user_id>
Sort Key (SK):      NOTIF#<timestamp>#<notification_id>

属性:
  - pk          (S): "USER#<user_id>"
  - sk          (S): "NOTIF#<2024-01-15T10:30:00Z>#<notif_id>"
  - type        (S): "message" | "friend_request" | "room_invite"
  - title       (S): "通知タイトル"
  - body        (S): "通知本文"
  - is_read     (BOOL): false
  - reference_id(S): "<room_id or user_id>"
  - created_at  (S): ISO 8601
  - ttl         (N): Unix timestamp (90日後に自動削除)
```

**クエリ例**:
```
# パターン4: ユーザーの通知一覧（新しい順）
PK = "USER#user123", SK begins_with "NOTIF#", ScanIndexForward=false
```

## PostgreSQL → DynamoDB 移行戦略 (Phase 4)

### 移行手順

1. **DynamoDB テーブル作成**: Terraform でテーブル・GSI を作成
2. **デュアルライト実装**: 新規データを PostgreSQL と DynamoDB の両方に書き込み
3. **データ移行スクリプト**: 既存データを DynamoDB へバッチ移行
4. **読み取り切り替え**: 読み取り先を DynamoDB に変更
5. **PostgreSQL 書き込み停止**: PostgreSQL への書き込みを停止
6. **PostgreSQL 廃止**: バックアップ後にデータベースを廃止

### Repository パターンによる抽象化

```go
// データストアに依存しないインターフェース
type MessageRepository interface {
    Create(ctx context.Context, msg *Message) error
    GetByRoomID(ctx context.Context, roomID string, limit int, cursor string) ([]*Message, string, error)
    GetByID(ctx context.Context, id string) (*Message, error)
}

// Phase 1-3: PostgreSQL 実装
type PostgresMessageRepository struct { ... }

// Phase 4: DynamoDB 実装
type DynamoDBMessageRepository struct { ... }
```

## Redis データモデル

```
# プレゼンス情報（Realtime Service）
presence:<user_id>  → {"status": "online", "last_seen": "..."}
                      TTL: 300秒（ハートビート更新）

# WebSocket 接続マッピング
ws:connections:<user_id> → Set of <server_id>:<connection_id>

# Pub/Sub チャンネル
channel:room:<room_id>  → メッセージ配信
channel:user:<user_id>  → 個人通知
```

## 関連ドキュメント

- [DynamoDB 設計詳細](../aws/dynamodb-design.md)
- [API 設計](./api-design.md)
