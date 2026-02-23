# API 設計

## REST API（API Gateway 経由）

### 認証

すべての API リクエストには `Authorization: Bearer <JWT>` ヘッダーが必要（一部を除く）。

### User Service エンドポイント

| メソッド | パス | 説明 | 認証 |
|---------|------|------|------|
| POST | `/api/v1/auth/register` | ユーザー登録 | 不要 |
| POST | `/api/v1/auth/login` | ログイン | 不要 |
| POST | `/api/v1/auth/refresh` | トークンリフレッシュ | 不要 |
| GET | `/api/v1/users/me` | 自分のプロフィール取得 | 必要 |
| PUT | `/api/v1/users/me` | プロフィール更新 | 必要 |
| GET | `/api/v1/users/:id` | ユーザー情報取得 | 必要 |
| GET | `/api/v1/users/search?q=` | ユーザー検索 | 必要 |
| GET | `/api/v1/friends` | フレンド一覧 | 必要 |
| POST | `/api/v1/friends/request` | フレンド申請 | 必要 |
| PUT | `/api/v1/friends/:id/accept` | フレンド承認 | 必要 |
| DELETE | `/api/v1/friends/:id` | フレンド解除 | 必要 |

### Chat Service エンドポイント

| メソッド | パス | 説明 | 認証 |
|---------|------|------|------|
| POST | `/api/v1/rooms` | ルーム作成 | 必要 |
| GET | `/api/v1/rooms` | 参加ルーム一覧 | 必要 |
| GET | `/api/v1/rooms/:id` | ルーム詳細 | 必要 |
| PUT | `/api/v1/rooms/:id` | ルーム情報更新 | 必要 |
| POST | `/api/v1/rooms/:id/members` | メンバー追加 | 必要 |
| DELETE | `/api/v1/rooms/:id/members/:userId` | メンバー削除 | 必要 |
| GET | `/api/v1/rooms/:id/messages` | メッセージ一覧 | 必要 |
| POST | `/api/v1/rooms/:id/messages` | メッセージ送信 | 必要 |
| PUT | `/api/v1/rooms/:id/messages/:msgId` | メッセージ編集 | 必要 |
| DELETE | `/api/v1/rooms/:id/messages/:msgId` | メッセージ削除 | 必要 |
| POST | `/api/v1/rooms/:id/read` | 既読を送信 | 必要 |

### Notification Service エンドポイント

| メソッド | パス | 説明 | 認証 |
|---------|------|------|------|
| GET | `/api/v1/notifications` | 通知一覧 | 必要 |
| PUT | `/api/v1/notifications/:id/read` | 通知を既読 | 必要 |
| PUT | `/api/v1/notifications/read-all` | 全通知を既読 | 必要 |
| GET | `/api/v1/notifications/unread-count` | 未読数取得 | 必要 |

### Media Service エンドポイント

| メソッド | パス | 説明 | 認証 |
|---------|------|------|------|
| POST | `/api/v1/media/upload-url` | Presigned URL 取得 | 必要 |
| POST | `/api/v1/media/upload-complete` | アップロード完了通知 | 必要 |
| GET | `/api/v1/media/:id` | メディアメタデータ取得 | 必要 |

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

## gRPC サービス定義

### User Service (proto/user/v1/user.proto)

```protobuf
syntax = "proto3";
package user.v1;

import "google/protobuf/timestamp.proto";

service UserService {
  // ユーザー管理
  rpc CreateUser(CreateUserRequest) returns (CreateUserResponse);
  rpc GetUser(GetUserRequest) returns (GetUserResponse);
  rpc UpdateUser(UpdateUserRequest) returns (UpdateUserResponse);
  rpc SearchUsers(SearchUsersRequest) returns (SearchUsersResponse);

  // フレンド管理
  rpc ListFriends(ListFriendsRequest) returns (ListFriendsResponse);
  rpc SendFriendRequest(SendFriendRequestReq) returns (SendFriendRequestResp);
  rpc AcceptFriendRequest(AcceptFriendRequestReq) returns (AcceptFriendRequestResp);

  // プレゼンス
  rpc GetUserPresence(GetUserPresenceRequest) returns (GetUserPresenceResponse);
}

message User {
  string id = 1;
  string email = 2;
  string username = 3;
  string display_name = 4;
  string avatar_url = 5;
  string status_text = 6;
  bool is_online = 7;
  google.protobuf.Timestamp created_at = 8;
  google.protobuf.Timestamp updated_at = 9;
}

message CreateUserRequest {
  string email = 1;
  string username = 2;
  string display_name = 3;
  string cognito_sub = 4;
}

message CreateUserResponse {
  User user = 1;
}

message GetUserRequest {
  string user_id = 1;
}

message GetUserResponse {
  User user = 1;
}

message UpdateUserRequest {
  string user_id = 1;
  optional string display_name = 2;
  optional string avatar_url = 3;
  optional string status_text = 4;
}

message UpdateUserResponse {
  User user = 1;
}

message SearchUsersRequest {
  string query = 1;
  int32 limit = 2;
  string cursor = 3;
}

message SearchUsersResponse {
  repeated User users = 1;
  string next_cursor = 2;
}

message ListFriendsRequest {
  string user_id = 1;
}

message ListFriendsResponse {
  repeated User friends = 1;
}

message SendFriendRequestReq {
  string user_id = 1;
  string friend_id = 2;
}

message SendFriendRequestResp {}

message AcceptFriendRequestReq {
  string user_id = 1;
  string friend_id = 2;
}

message AcceptFriendRequestResp {}

message GetUserPresenceRequest {
  repeated string user_ids = 1;
}

message GetUserPresenceResponse {
  map<string, bool> presence = 1;
}
```

### Chat Service (proto/chat/v1/chat.proto)

```protobuf
syntax = "proto3";
package chat.v1;

import "google/protobuf/timestamp.proto";

service ChatService {
  // ルーム管理
  rpc CreateRoom(CreateRoomRequest) returns (CreateRoomResponse);
  rpc GetRoom(GetRoomRequest) returns (GetRoomResponse);
  rpc ListRooms(ListRoomsRequest) returns (ListRoomsResponse);
  rpc AddMember(AddMemberRequest) returns (AddMemberResponse);
  rpc RemoveMember(RemoveMemberRequest) returns (RemoveMemberResponse);

  // メッセージ管理
  rpc SendMessage(SendMessageRequest) returns (SendMessageResponse);
  rpc GetMessages(GetMessagesRequest) returns (GetMessagesResponse);
  rpc EditMessage(EditMessageRequest) returns (EditMessageResponse);
  rpc DeleteMessage(DeleteMessageRequest) returns (DeleteMessageResponse);

  // 既読管理
  rpc MarkAsRead(MarkAsReadRequest) returns (MarkAsReadResponse);
}

enum RoomType {
  ROOM_TYPE_UNSPECIFIED = 0;
  ROOM_TYPE_DIRECT = 1;
  ROOM_TYPE_GROUP = 2;
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
  RoomType type = 3;
  string created_by = 4;
  repeated RoomMember members = 5;
  google.protobuf.Timestamp created_at = 6;
}

message RoomMember {
  string user_id = 1;
  string role = 2;
  google.protobuf.Timestamp joined_at = 3;
}

message Message {
  string id = 1;
  string room_id = 2;
  string sender_id = 3;
  string content = 4;
  MessageType message_type = 5;
  string media_url = 6;
  string parent_id = 7;
  bool is_edited = 8;
  google.protobuf.Timestamp created_at = 9;
  google.protobuf.Timestamp updated_at = 10;
}

message CreateRoomRequest {
  string name = 1;
  RoomType type = 2;
  repeated string member_ids = 3;
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
  string user_id = 1;
  int32 limit = 2;
  string cursor = 3;
}

message ListRoomsResponse {
  repeated Room rooms = 1;
  string next_cursor = 2;
}

message AddMemberRequest {
  string room_id = 1;
  string user_id = 2;
}

message AddMemberResponse {}

message RemoveMemberRequest {
  string room_id = 1;
  string user_id = 2;
}

message RemoveMemberResponse {}

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

message EditMessageRequest {
  string message_id = 1;
  string sender_id = 2;
  string content = 3;
}

message EditMessageResponse {
  Message message = 1;
}

message DeleteMessageRequest {
  string message_id = 1;
  string sender_id = 2;
}

message DeleteMessageResponse {}

message MarkAsReadRequest {
  string room_id = 1;
  string user_id = 2;
  string message_id = 3;
}

message MarkAsReadResponse {}
```

### Realtime Service (proto/realtime/v1/realtime.proto)

```protobuf
syntax = "proto3";
package realtime.v1;

import "google/protobuf/timestamp.proto";

service RealtimeService {
  // サーバーからクライアントへのイベントストリーム
  rpc Subscribe(SubscribeRequest) returns (stream RealtimeEvent);

  // プレゼンス管理
  rpc UpdatePresence(UpdatePresenceRequest) returns (UpdatePresenceResponse);

  // タイピングインジケーター
  rpc SendTyping(SendTypingRequest) returns (SendTypingResponse);
}

message SubscribeRequest {
  string user_id = 1;
  repeated string room_ids = 2;
}

enum EventType {
  EVENT_TYPE_UNSPECIFIED = 0;
  EVENT_TYPE_NEW_MESSAGE = 1;
  EVENT_TYPE_MESSAGE_EDITED = 2;
  EVENT_TYPE_MESSAGE_DELETED = 3;
  EVENT_TYPE_USER_JOINED = 4;
  EVENT_TYPE_USER_LEFT = 5;
  EVENT_TYPE_TYPING = 6;
  EVENT_TYPE_PRESENCE_CHANGED = 7;
  EVENT_TYPE_READ_RECEIPT = 8;
}

message RealtimeEvent {
  EventType type = 1;
  string room_id = 2;
  string user_id = 3;
  bytes payload = 4;
  google.protobuf.Timestamp timestamp = 5;
}

message UpdatePresenceRequest {
  string user_id = 1;
  bool is_online = 2;
}

message UpdatePresenceResponse {}

message SendTypingRequest {
  string user_id = 1;
  string room_id = 2;
}

message SendTypingResponse {}
```

### Notification Service (proto/notification/v1/notification.proto)

```protobuf
syntax = "proto3";
package notification.v1;

import "google/protobuf/timestamp.proto";

service NotificationService {
  rpc GetNotifications(GetNotificationsRequest) returns (GetNotificationsResponse);
  rpc MarkAsRead(MarkNotificationReadRequest) returns (MarkNotificationReadResponse);
  rpc MarkAllAsRead(MarkAllReadRequest) returns (MarkAllReadResponse);
  rpc GetUnreadCount(GetUnreadCountRequest) returns (GetUnreadCountResponse);
}

enum NotificationType {
  NOTIFICATION_TYPE_UNSPECIFIED = 0;
  NOTIFICATION_TYPE_MESSAGE = 1;
  NOTIFICATION_TYPE_FRIEND_REQUEST = 2;
  NOTIFICATION_TYPE_ROOM_INVITE = 3;
}

message Notification {
  string id = 1;
  string user_id = 2;
  NotificationType type = 3;
  string title = 4;
  string body = 5;
  bool is_read = 6;
  string reference_id = 7;
  google.protobuf.Timestamp created_at = 8;
}

message GetNotificationsRequest {
  string user_id = 1;
  int32 limit = 2;
  string cursor = 3;
}

message GetNotificationsResponse {
  repeated Notification notifications = 1;
  string next_cursor = 2;
  bool has_more = 3;
}

message MarkNotificationReadRequest {
  string notification_id = 1;
}

message MarkNotificationReadResponse {}

message MarkAllReadRequest {
  string user_id = 1;
}

message MarkAllReadResponse {}

message GetUnreadCountRequest {
  string user_id = 1;
}

message GetUnreadCountResponse {
  int32 count = 1;
}
```

### Media Service (proto/media/v1/media.proto)

```protobuf
syntax = "proto3";
package media.v1;

import "google/protobuf/timestamp.proto";

service MediaService {
  rpc GetUploadURL(GetUploadURLRequest) returns (GetUploadURLResponse);
  rpc CompleteUpload(CompleteUploadRequest) returns (CompleteUploadResponse);
  rpc GetMedia(GetMediaRequest) returns (GetMediaResponse);
}

message GetUploadURLRequest {
  string user_id = 1;
  string file_name = 2;
  string content_type = 3;
  int64 file_size = 4;
}

message GetUploadURLResponse {
  string upload_url = 1;
  string media_id = 2;
  string object_key = 3;
}

message CompleteUploadRequest {
  string media_id = 1;
  string user_id = 2;
}

message CompleteUploadResponse {
  MediaInfo media = 1;
}

message GetMediaRequest {
  string media_id = 1;
}

message GetMediaResponse {
  MediaInfo media = 1;
}

message MediaInfo {
  string id = 1;
  string user_id = 2;
  string file_name = 3;
  string content_type = 4;
  int64 file_size = 5;
  string url = 6;
  string thumbnail_url = 7;
  google.protobuf.Timestamp created_at = 8;
}
```

---

## WebSocket メッセージフォーマット

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

// タイピング開始
{
  "type": "typing",
  "data": {
    "room_id": "room_abc"
  }
}

// 既読送信
{
  "type": "read",
  "data": {
    "room_id": "room_abc",
    "message_id": "msg_123"
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

// タイピング通知
{
  "type": "typing",
  "data": {
    "room_id": "room_abc",
    "user_id": "user_789"
  }
}

// プレゼンス変更
{
  "type": "presence",
  "data": {
    "user_id": "user_789",
    "is_online": true
  }
}

// 既読通知
{
  "type": "read_receipt",
  "data": {
    "room_id": "room_abc",
    "user_id": "user_789",
    "message_id": "msg_123"
  }
}

// メッセージ編集
{
  "type": "message_edited",
  "data": {
    "id": "msg_456",
    "room_id": "room_abc",
    "content": "Hello! (edited)",
    "updated_at": "2024-01-15T10:35:00Z"
  }
}

// メッセージ削除
{
  "type": "message_deleted",
  "data": {
    "id": "msg_456",
    "room_id": "room_abc"
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

- [アーキテクチャ概要](./overview.md)
- [マイクロサービス詳細](./microservices.md)
- [データモデル設計](./data-model.md)
