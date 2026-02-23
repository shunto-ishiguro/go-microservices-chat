# DynamoDB 設計詳細

## 設計アプローチの選択

### Single-Table Design vs Multi-Table Design

| 方式 | メリット | デメリット |
|------|---------|-----------|
| **Single-Table** | クエリ効率が高い、RCU/WCU 共有 | 設計が複雑、可読性が低い |
| **Multi-Table** | シンプル、理解しやすい | テーブルごとにキャパシティ管理が必要 |

**本プロジェクトの選択: Multi-Table Design（サービスごとにテーブル分離）**

理由:
1. **Database-per-Service パターン**との整合性（各サービスが自分のテーブルを所有）
2. 学習初期段階では**シンプルさ**を優先
3. サービスごとに独立してスケール可能
4. ただしテーブル内では**アイテムコレクション**パターンを活用（rooms テーブルなど）

---

## テーブル一覧

| テーブル名 | 所有サービス | 用途 |
|-----------|------------|------|
| `chat-messages` | Chat Service | メッセージ保存 |
| `chat-rooms` | Chat Service | ルーム・メンバー・既読管理 |
| `notifications` | Notification Service | 通知履歴 |

---

## パーティションキー設計

### チャットワークロードの特性

チャットアプリケーションのアクセスパターンは以下の特性がある:

1. **ルーム単位のアクセスが大半**: メッセージの読み書きはルーム ID ごと
2. **最新データへのアクセスが多い**: 古いメッセージはほとんど読まれない
3. **ホットパーティション**: 人気のグループチャットにアクセスが集中

### パーティションキー設計原則

```
良い PK: アクセスが均等に分散される
  → ROOM#<room_id>   ✓ ルームごとに分散
  → USER#<user_id>   ✓ ユーザーごとに分散

悪い PK: アクセスが一部に集中する
  → DATE#2024-01-15  ✗ 当日にアクセス集中
  → STATUS#active    ✗ 大半が active
```

### ホットパーティション対策

大規模グループチャットでパーティションが過熱する場合の対策:

```
方式1: Write Sharding（書き込み分散）
  PK: ROOM#<room_id>#SHARD#<0-9>
  → 10個のパーティションに書き込みを分散
  → 読み取り時は全シャードを並列クエリ

方式2: GSI の活用
  → 異なるアクセスパターンに最適化された GSI を作成
```

> **注意**: 本プロジェクトの規模ではホットパーティション問題は発生しない可能性が高いが、設計知識として理解しておく。

---

## テーブル詳細設計

### chat-messages テーブル

**目的**: メッセージの保存と取得

```
Table: chat-messages
  PK: pk (String)  = "ROOM#<room_id>"
  SK: sk (String)  = "MSG#<iso_timestamp>#<message_id>"

  Attributes:
    message_id    (S)  : UUID
    sender_id     (S)  : ユーザー ID
    content       (S)  : メッセージ本文
    message_type  (S)  : "text" | "image" | "file"
    media_url     (S)  : メディア URL（オプション）
    parent_id     (S)  : 返信先メッセージ ID（オプション）
    is_edited     (BOOL): 編集済みフラグ
    is_deleted    (BOOL): 削除済みフラグ
    created_at    (S)  : ISO 8601
    updated_at    (S)  : ISO 8601

  GSI1 (sender-index):
    PK: gsi1pk (S) = "USER#<sender_id>"
    SK: gsi1sk (S) = "MSG#<iso_timestamp>"
    用途: 特定ユーザーの送信メッセージ検索
```

**アクセスパターン**:

| パターン | キー条件 | 備考 |
|---------|---------|------|
| ルームのメッセージ取得（新しい順） | PK=`ROOM#<id>`, SK begins_with `MSG#`, ScanIndexForward=false | Limit でページネーション |
| ルームの最新 N 件取得 | PK=`ROOM#<id>`, SK begins_with `MSG#`, ScanIndexForward=false, Limit=N | - |
| 特定時刻以降のメッセージ | PK=`ROOM#<id>`, SK > `MSG#<timestamp>` | 差分取得 |
| ユーザーの送信履歴 | GSI1: PK=`USER#<id>` | 管理用 |

**SK の設計ポイント**:
```
SK = "MSG#2024-01-15T10:30:00Z#550e8400-e29b-41d4"
         ↑ ISO 8601 タイムスタンプ    ↑ UUID（同一時刻の一意性保証）

・タイムスタンプを先頭にすることで時間順のソートが可能
・ScanIndexForward=false で新しい順に取得
・UUID を付加して同一ミリ秒のメッセージも一意に識別
```

### chat-rooms テーブル

**目的**: ルーム情報、メンバー、既読状態を1テーブルで管理（アイテムコレクション）

```
Table: chat-rooms
  PK: pk (String)  = "ROOM#<room_id>"
  SK: sk (String)  = "METADATA" | "MEMBER#<user_id>" | "READ#<user_id>"

  --- アイテムタイプ: ルームメタデータ ---
  PK: "ROOM#<room_id>"
  SK: "METADATA"
    name        (S)  : ルーム名
    type        (S)  : "direct" | "group"
    created_by  (S)  : 作成者 ID
    member_count(N)  : メンバー数
    created_at  (S)  : ISO 8601
    updated_at  (S)  : ISO 8601

  --- アイテムタイプ: メンバー ---
  PK: "ROOM#<room_id>"
  SK: "MEMBER#<user_id>"
    gsi1pk    (S)  : "USER#<user_id>"
    gsi1sk    (S)  : "ROOM#<room_id>"
    role      (S)  : "owner" | "admin" | "member"
    joined_at (S)  : ISO 8601

  --- アイテムタイプ: 既読状態 ---
  PK: "ROOM#<room_id>"
  SK: "READ#<user_id>"
    last_read_msg_id (S)  : 最後に読んだメッセージ ID
    last_read_at     (S)  : ISO 8601

  GSI1 (user-rooms-index):
    PK: gsi1pk (S) = "USER#<user_id>"
    SK: gsi1sk (S) = "ROOM#<room_id>"
    用途: ユーザーの参加ルーム一覧
```

**アクセスパターン**:

| パターン | キー条件 | 備考 |
|---------|---------|------|
| ルーム情報取得 | PK=`ROOM#<id>`, SK=`METADATA` | GetItem |
| メンバー一覧 | PK=`ROOM#<id>`, SK begins_with `MEMBER#` | Query |
| 既読状態取得 | PK=`ROOM#<id>`, SK=`READ#<user_id>` | GetItem |
| ユーザーの参加ルーム | GSI1: PK=`USER#<id>`, SK begins_with `ROOM#` | Query |
| ルーム全情報 | PK=`ROOM#<id>` | Query（メタデータ+メンバー+既読を一括取得） |

### notifications テーブル

```
Table: notifications
  PK: pk (String)  = "USER#<user_id>"
  SK: sk (String)  = "NOTIF#<iso_timestamp>#<notification_id>"

  Attributes:
    notification_id (S)  : UUID
    type            (S)  : "message" | "friend_request" | "room_invite"
    title           (S)  : 通知タイトル
    body            (S)  : 通知本文
    is_read         (BOOL): 既読フラグ
    reference_id    (S)  : 関連リソース ID
    created_at      (S)  : ISO 8601
    ttl             (N)  : Unix timestamp（TTL による自動削除）
```

**アクセスパターン**:

| パターン | キー条件 | 備考 |
|---------|---------|------|
| 通知一覧（新しい順） | PK=`USER#<id>`, SK begins_with `NOTIF#`, ScanIndexForward=false | Limit でページネーション |
| 特定通知の既読更新 | PK=`USER#<id>`, SK=`NOTIF#<ts>#<id>` | UpdateItem |

**TTL 設計**:
- 90 日後の Unix timestamp を `ttl` 属性に設定
- DynamoDB が自動的に期限切れアイテムを削除（コスト削減）

---

## GSI 戦略

### GSI 設計原則

1. **アクセスパターンから逆算**: 必要なクエリに基づいて GSI を設計
2. **GSI の数を最小限に**: 各 GSI は追加の WCU/RCU を消費する
3. **Sparse Index の活用**: GSI キーが存在するアイテムのみインデックス化

### 本プロジェクトの GSI

| テーブル | GSI 名 | PK | SK | 用途 |
|---------|--------|----|----|------|
| chat-messages | sender-index | `USER#<user_id>` | `MSG#<timestamp>` | ユーザーの送信履歴 |
| chat-rooms | user-rooms-index | `USER#<user_id>` | `ROOM#<room_id>` | ユーザーの参加ルーム |

---

## キャパシティモード

### 環境別の推奨設定

| 環境 | モード | 理由 |
|------|-------|------|
| dev | On-Demand | トラフィックが不規則、コスト最小化 |
| staging | On-Demand | テスト時のみトラフィック発生 |
| prod | Provisioned + Auto Scaling | 安定したトラフィック、コスト予測可能 |

### Provisioned キャパシティの目安（prod）

```
chat-messages:
  RCU: 100 (Auto Scaling: 50-500)
  WCU: 50  (Auto Scaling: 25-250)

chat-rooms:
  RCU: 50 (Auto Scaling: 25-250)
  WCU: 25 (Auto Scaling: 10-100)

notifications:
  RCU: 30 (Auto Scaling: 15-150)
  WCU: 20 (Auto Scaling: 10-100)
```

---

## Go での DynamoDB 操作例

```go
package repository

import (
    "context"
    "fmt"
    "time"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type DynamoDBMessageRepository struct {
    client    *dynamodb.Client
    tableName string
}

// ルームのメッセージを取得（新しい順）
func (r *DynamoDBMessageRepository) GetByRoomID(
    ctx context.Context,
    roomID string,
    limit int,
    cursor string,
) ([]*Message, string, error) {
    input := &dynamodb.QueryInput{
        TableName:              &r.tableName,
        KeyConditionExpression: aws.String("pk = :pk AND begins_with(sk, :prefix)"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":pk":     &types.AttributeValueMemberS{Value: fmt.Sprintf("ROOM#%s", roomID)},
            ":prefix": &types.AttributeValueMemberS{Value: "MSG#"},
        },
        ScanIndexForward: aws.Bool(false), // 新しい順
        Limit:            aws.Int32(int32(limit)),
    }

    // カーソルベースページネーション
    if cursor != "" {
        // cursor をデコードして ExclusiveStartKey に設定
    }

    result, err := r.client.Query(ctx, input)
    if err != nil {
        return nil, "", err
    }

    var messages []*Message
    err = attributevalue.UnmarshalListOfMaps(result.Items, &messages)
    // ...
    return messages, nextCursor, nil
}
```

## 関連ドキュメント

- [データモデル設計](../architecture/data-model.md)
- [AWS サービス一覧](./services.md)
- [API 設計](../architecture/api-design.md)
