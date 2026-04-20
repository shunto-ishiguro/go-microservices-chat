# API 設計

## 全体方針

このプロジェクトでは **gRPC を API の一次ソース** とする。REST は **infra 側 Envoy (Envoy gRPC-JSON Transcoder / grpc-gateway 等)** が proto 定義 (`google.api.http` アノテーション付き) から自動変換する想定。このリポジトリ (app) は gRPC だけを公開する。

- bufconn ユニットテスト: `grpcurl -H "x-user-id: <uuid>"` 相当の metadata 直接注入で検証 (app は JWT 検証しない、`x-user-id` を信じるだけ)
- REST 公開: infra 側 Envoyが Transcoder を有効化した時点で、proto の `google.api.http` アノテーションから自動公開される

したがって **REST エンドポイント一覧は infra 側 Envoyが公開するクライアント向け API の仕様** を示すものであり、user-service / chat-service 自体は REST サーバーを持たない。

---

## クライアントから見たトランスポート

ブラウザ等のクライアントは **REST と WebSocket の 2 つの入口** を使い分ける。決め方のルールは以下:

| 性質 | 使うトランスポート |
|------|-------------------|
| 参照・一覧 (ルーム一覧、ルーム検索、メッセージ履歴 等) | **REST** |
| リソース管理 (ルーム作成、メンバー追加・削除、プロフィール更新 等) | **REST** |
| 認証 (login / register / refresh / logout) | **REST** |
| **リアルタイム送信** (メッセージ送信、タイピング等) | **WebSocket** |
| **リアルタイム受信** (新着メッセージ 等の push) | **WebSocket** |

### アクション別マトリクス

| クライアント操作 | 入口 | 内部経路 |
|-----------------|------|---------|
| サインアップ / ログイン | REST | Gateway → user-service (gRPC `Register`/`Login`) |
| 自分のプロフィール取得・更新 | REST | Gateway → user-service (gRPC `GetMe`/`UpdateMe`) |
| 公開ルームの検索 | REST | Gateway → chat-service (gRPC `SearchRooms`) |
| 自分の参加ルーム一覧 | REST | Gateway → chat-service (gRPC `ListRooms`) |
| ルーム作成 / 詳細取得 (ヘッダのみ) | REST | Gateway → chat-service (gRPC `CreateRoom`/`GetRoom`) |
| ルームのメンバー一覧 | REST | Gateway → chat-service (gRPC `ListRoomMembers`) → 内部で user-service `BatchGetUsers` で enrich |
| ルーム参加 / 退出 | REST | Gateway → chat-service (gRPC `JoinRoom`/`LeaveRoom`) |
| メッセージ履歴取得 | REST | Gateway → chat-service (gRPC `GetMessages`) |
| **メッセージ送信** | **WebSocket** | realtime-service → chat-service (gRPC `SendMessage`) + Redis PUBLISH |
| 新着メッセージの受信 | **WebSocket (push)** | Redis SUBSCRIBE → realtime-service → WebSocket |

> **なぜ送信は WebSocket なのか**: REST (POST /messages) も技術的には可能だが、**すでに張ってある WebSocket** に相乗りする方が低遅延・省リソース。さらに、realtime-service が受信 → 即座に Redis にも PUBLISH できるので配信遅延を最小化できる。

---

## REST API (infra 側 Envoyの Transcoder / grpc-gateway で自動公開)

> このセクションに載っているのは **REST で公開されるエンドポイントのみ**。メッセージ送信はここには出てこない (WebSocket の [WebSocket メッセージフォーマット](#websocket-メッセージフォーマット) 参照)。

### 認証

すべての API リクエストには `Authorization: Bearer <JWT>` ヘッダーが必要（一部を除く）。

### User Service エンドポイント

| メソッド | パス | 対応 gRPC | 画面 / 操作 | 認証 |
|---------|------|-----------|-----------|------|
| POST | `/api/v1/auth/register` | `Register` | 画面 #2「新規登録」ボタン | 不要 |
| POST | `/api/v1/auth/login` | `Login` | 画面 #1「ログイン」ボタン | 不要 |
| POST | `/api/v1/auth/refresh` | `Refresh` | クライアントの自動トークン更新 | 不要 |
| GET | `/api/v1/users/me` | `GetMe` | 画面 #7 マウント時 | 必要 |
| PUT | `/api/v1/users/me` | `UpdateMe` | 画面 #7「保存」ボタン | 必要 |
| GET | `/api/v1/users/:id` | `GetUser` | 画面 #8 マウント時 (メンバー詳細) | 必要 |

> `BatchGetUsers([]user_ids)` は **内部 RPC** (chat-service のメンバー enrich 用、N+1 回避)。REST 公開しない。[内部 gRPC](#内部-grpc-rest-非公開) 参照。

### Chat Service エンドポイント

| メソッド | パス | 対応 gRPC | 画面 / 操作 | 認証 |
|---------|------|-----------|-----------|------|
| POST | `/api/v1/rooms` | `CreateRoom` | 画面 #4「作成」ボタン | 必要 |
| GET | `/api/v1/rooms` | `ListRooms` | 画面 #5 マウント時 (自分の参加ルーム) | 必要 |
| GET | `/api/v1/rooms/search?q=` | `SearchRooms` | 画面 #3 マウント時 (公開ルーム検索) | 必要 |
| GET | `/api/v1/rooms/:id` | `GetRoom` | 画面 #6 マウント時 (ヘッダ軽量情報のみ) | 必要 |
| GET | `/api/v1/rooms/:id/members` | `ListRoomMembers` | 画面 #9 マウント時 (メンバー一覧、enrich 済み) | 必要 |
| POST | `/api/v1/rooms/:id/join` | `JoinRoom` | 画面 #3「参加」ボタン | 必要 |
| DELETE | `/api/v1/rooms/:id/members/me` | `LeaveRoom` | 画面 #6「退出」ボタン | 必要 |
| GET | `/api/v1/rooms/:id/messages` | `GetMessages` (Phase 2) | 画面 #6 マウント時 (履歴) | 必要 |

> **メッセージ送信は REST では公開しない**。クライアントは WebSocket で `send_message` を送る (後述)。

### 内部 gRPC (REST 非公開)

クライアントから直接叩かれず、app サービス間でのみ呼ばれる RPC。`google.api.http` アノテーションを付けないので Transcoder の REST 自動生成対象外。

| gRPC | 呼び出し元 | 用途 |
|------|----------|------|
| `user.v1.UserService.BatchGetUsers([]user_ids)` | chat-service | `GetRoom` のメンバー一覧を 1 回で enrich (N+1 回避) |
| `chat.v1.ChatService.SendMessage` (Phase 2) | realtime-service | WebSocket 受信メッセージの永続化 |

> **全ルームは public**。誰でも検索・参加できる。招待・追放・プライベートルーム機能は持たない。

### 共通レスポンスフォーマット

```json
// 成功レスポンス
{
  "data": { ... },
  "meta": {
    "request_id": "req_abc123"
  }
}

// ページネーション付きレスポンス
{
  "data": [ ... ],
  "meta": {
    "request_id": "req_abc123",
    "pagination": {
      "next_cursor": "eyJsYXN0X2lkIjoiMTIzIn0=",
      "has_more": true
    }
  }
}

// エラーレスポンス
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid email format",
    "details": [
      {
        "field": "email",
        "message": "must be a valid email address"
      }
    ]
  },
  "meta": {
    "request_id": "req_abc123"
  }
}
```

### HTTP ステータスコード

| コード | 意味 | 使用場面 |
|-------|------|---------|
| 200 | OK | 取得・更新成功 |
| 201 | Created | リソース作成成功 |
| 204 | No Content | 削除成功 |
| 400 | Bad Request | バリデーションエラー |
| 401 | Unauthorized | 認証エラー |
| 403 | Forbidden | 権限エラー |
| 404 | Not Found | リソース未存在 |
| 409 | Conflict | 重複エラー |
| 429 | Too Many Requests | レート制限超過 |
| 500 | Internal Server Error | サーバーエラー |

---

## gRPC サービス定義（API の一次ソース）

**Phase 1 で user-service はこの gRPC 定義で実装**。bufconn ユニットテストで動作確認し、infra 側 Envoyの Transcoder 経由で REST 公開する。

> **ここの RPC 全てが REST で公開されるわけではない**。`chat.v1.SendMessage` は **REST には公開しない** (`google.api.http` アノテーションを付けない)。クライアント → WebSocket → realtime-service → chat-service の経路で **内部 gRPC として呼ばれる** だけ。



### User Service (proto/user/v1/user.proto)

```protobuf
syntax = "proto3";
package user.v1;

import "google/protobuf/timestamp.proto";

service UserService {
  // 認証 (REST 公開)
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc Login(LoginRequest) returns (LoginResponse);
  rpc Refresh(RefreshRequest) returns (RefreshResponse);

  // 自分のプロフィール (REST 公開、画面 #7)
  //   対象 ID は x-user-id metadata から解決する。user_id を引数に取らない。
  rpc GetMe(GetMeRequest) returns (GetMeResponse);
  rpc UpdateMe(UpdateMeRequest) returns (UpdateMeResponse);

  // 他ユーザー 1 件取得 (REST 公開、画面 #8 メンバー詳細)
  rpc GetUser(GetUserRequest) returns (GetUserResponse);

  // 他ユーザー N 件一括取得 (内部 RPC、REST 非公開)
  //   chat-service のメンバー enrich で使う。N+1 を回避するためのバッチ。
  rpc BatchGetUsers(BatchGetUsersRequest) returns (BatchGetUsersResponse);
}

message User {
  string id = 1;
  string email = 2;
  string username = 3;
  string display_name = 4;
  string avatar_url = 5;
  string status_text = 6;
  google.protobuf.Timestamp created_at = 7;
  google.protobuf.Timestamp updated_at = 8;
}

message RegisterRequest {
  string email = 1;
  string username = 2;
  string display_name = 3;
  string password = 4;
}

message RegisterResponse {
  User user = 1;
}

message LoginRequest {
  string email = 1;
  string password = 2;
}

message LoginResponse {
  string access_token = 1;
  string refresh_token = 2;
}

message RefreshRequest {
  string refresh_token = 1;
}

message RefreshResponse {
  string access_token = 1;
  string refresh_token = 2;
}

message GetMeRequest {}

message GetMeResponse {
  User user = 1;
}

message UpdateMeRequest {
  optional string display_name = 1;
  optional string avatar_url = 2;
  optional string status_text = 3;
}

message UpdateMeResponse {
  User user = 1;
}

message GetUserRequest {
  string user_id = 1;
}

message GetUserResponse {
  User user = 1;
}

message BatchGetUsersRequest {
  repeated string user_ids = 1;
}

message BatchGetUsersResponse {
  // 存在しない ID は結果から欠落する (エラーにはしない)。
  repeated User users = 1;
}
```

### Chat Service (proto/chat/v1/chat.proto)

```protobuf
syntax = "proto3";
package chat.v1;

import "google/protobuf/timestamp.proto";

service ChatService {
  // ルーム管理 (REST 公開)
  rpc CreateRoom(CreateRoomRequest) returns (CreateRoomResponse);
  rpc GetRoom(GetRoomRequest) returns (GetRoomResponse);             // ヘッダ軽量情報のみ
  rpc ListRooms(ListRoomsRequest) returns (ListRoomsResponse);       // 自分の参加ルーム
  rpc SearchRooms(SearchRoomsRequest) returns (SearchRoomsResponse); // 公開ルーム検索
  rpc JoinRoom(JoinRoomRequest) returns (JoinRoomResponse);
  rpc LeaveRoom(LeaveRoomRequest) returns (LeaveRoomResponse);
  rpc ListRoomMembers(ListRoomMembersRequest) returns (ListRoomMembersResponse); // 画面 #9、enrich 済み

  // メッセージ履歴 (REST 公開)
  rpc GetMessages(GetMessagesRequest) returns (GetMessagesResponse);

  // 内部 RPC (REST 公開しない、realtime-service から呼ばれる)
  rpc SendMessage(SendMessageRequest) returns (SendMessageResponse);
}

enum MessageType {
  MESSAGE_TYPE_UNSPECIFIED = 0;
  MESSAGE_TYPE_TEXT = 1;
  MESSAGE_TYPE_IMAGE = 2;
  MESSAGE_TYPE_FILE = 3;
}

message Room {
  string id = 1;
  string name = 2;
  string created_by = 3;
  int32 member_count = 4;
  google.protobuf.Timestamp created_at = 5;
  // メンバー一覧は ListRoomMembers で別途取得する。
}

message RoomMember {
  string user_id = 1;
  google.protobuf.Timestamp joined_at = 2;
  // ListRoomMembers のレスポンスで chat-service が enrich する軽量フィールド。
  // 詳細情報 (status_text 等) は画面 #8 で GetUser(user_id) を叩いて取る。
  string display_name = 3;
  string avatar_url = 4;
}

message Message {
  string id = 1;
  string room_id = 2;
  string sender_id = 3;
  string content = 4;
  MessageType message_type = 5;
  string media_url = 6;
  string parent_id = 7;
  google.protobuf.Timestamp created_at = 8;
}

message CreateRoomRequest {
  string name = 1;
}

message CreateRoomResponse {
  Room room = 1;
}

message GetRoomRequest {
  string room_id = 1;
}

message GetRoomResponse {
  Room room = 1;
}

message ListRoomsRequest {
  int32 limit = 1;
  string cursor = 2;
}

message ListRoomsResponse {
  repeated Room rooms = 1;
  string next_cursor = 2;
}

message SearchRoomsRequest {
  string query = 1;
  int32 limit = 2;
  string cursor = 3;
}

message SearchRoomsResponse {
  repeated Room rooms = 1;
  string next_cursor = 2;
}

message JoinRoomRequest {
  string room_id = 1;
}

message JoinRoomResponse {}

message LeaveRoomRequest {
  string room_id = 1;
}

message LeaveRoomResponse {}

message ListRoomMembersRequest {
  string room_id = 1;
  int32 limit = 2;
  string cursor = 3;
}

message ListRoomMembersResponse {
  repeated RoomMember members = 1;
  string next_cursor = 2;
}

message SendMessageRequest {
  string room_id = 1;
  string sender_id = 2;
  string content = 3;
  MessageType message_type = 4;
  string media_url = 5;
  string parent_id = 6;
}

message SendMessageResponse {
  Message message = 1;
}

message GetMessagesRequest {
  string room_id = 1;
  int32 limit = 2;
  string cursor = 3;
}

message GetMessagesResponse {
  repeated Message messages = 1;
  string next_cursor = 2;
  bool has_more = 3;
}
```

### Realtime Service

realtime-service は **gRPC サーバーを公開しない**。外部との通信は WebSocket、内部との通信は以下のとおり。

| 通信 | 方向 | プロトコル | 用途 |
|------|------|-----------|------|
| クライアント ↔ realtime-service | 双方向 | WebSocket | チャットメッセージの送受信 |
| realtime-service → chat-service | 片方向 | gRPC Unary (`SendMessage`) | 受信メッセージの永続化依頼 |
| realtime-service ↔ Redis | 双方向 | `PUBLISH` / `PSUBSCRIBE` | プロセス間の配信バス |

リアルタイム配信の経路は gRPC ではなく **Redis Pub/Sub** に集約しているため、`proto/realtime/v1/realtime.proto` は存在しない (Phase 2 の realtime-service 実装時も追加しない)。

WebSocket のメッセージ仕様は [WebSocket メッセージフォーマット](#websocket-メッセージフォーマット) 参照。

---

## WebSocket メッセージフォーマット

ブラウザから **realtime-service** に張るリアルタイム通信路。メッセージ送信・受信はすべてこの経路で行う。

- **通信相手**: クライアント ↔ realtime-service (`:8081`、実環境では infra 側 Envoy経由)
- **通信形式**: 接続は張りっぱなし。JSON メッセージを双方向に流す
- **内部処理**: realtime-service が受信したメッセージを chat-service (永続化) と Redis (配信) に流す。詳細は [realtime-message-flow.md](../flow/realtime-message-flow.md) 参照

### 接続

```
WSS /ws?token=<JWT>
```

### クライアント → サーバー メッセージ

```json
// ルーム購読
{
  "type": "subscribe",
  "data": {
    "room_ids": ["room_abc", "room_def"]
  }
}

// ルーム購読解除
{
  "type": "unsubscribe",
  "data": {
    "room_ids": ["room_abc"]
  }
}

// メッセージ送信
{
  "type": "send_message",
  "data": {
    "room_id": "room_abc",
    "content": "Hello!",
    "message_type": "text"
  }
}

// Ping (Keep-alive)
{
  "type": "ping"
}
```

### サーバー → クライアント メッセージ

```json
// 新規メッセージ
{
  "type": "new_message",
  "data": {
    "id": "msg_456",
    "room_id": "room_abc",
    "sender_id": "user_789",
    "content": "Hello!",
    "message_type": "text",
    "created_at": "2024-01-15T10:30:00Z"
  }
}

// エラー
{
  "type": "error",
  "data": {
    "code": "UNAUTHORIZED",
    "message": "Token expired"
  }
}

// Pong
{
  "type": "pong"
}
```

## 関連ドキュメント

- [マイクロサービス詳細](./microservices.md)
- [データモデル設計](./data-model.md)
