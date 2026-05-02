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

> **メッセージ送信は REST では公開しない**。クライアントは WebSocket で `{"type":"message","content":"..."}` を送る (後述の [WebSocket メッセージフォーマット](#websocket-メッセージフォーマット) 参照)。

### 内部 gRPC (REST 非公開)

クライアントから直接叩かれず、app サービス間でのみ呼ばれる RPC。`google.api.http` アノテーションを付けないので Transcoder の REST 自動生成対象外。

| gRPC | 呼び出し元 | 用途 |
|------|----------|------|
| `user.v1.UserService.BatchGetUsers([]user_ids)` | chat-service | `GetRoom` のメンバー一覧を 1 回で enrich (N+1 回避) |
| `chat.v1.ChatService.SendMessage` (Phase 2) | realtime-service | WebSocket 受信メッセージの永続化 |

> **全ルームは public**。誰でも検索・参加できる。招待・追放・プライベートルーム機能は持たない。

### レスポンスフォーマット

Envoy gRPC-JSON Transcoder のデフォルト挙動に従い、**proto をそのまま JSON シリアライズ** したものが返る。`{data, meta, error}` のような共通 envelope (= 全レスポンスを包む外側の枠) は **採用しない**。

> **なぜ envelope なしか**: 本プロジェクトは gRPC を一次ソースとし、proto 構造をそのまま REST に晒す方針 (Google Cloud / GitHub / Stripe 等の主要 API と同じスタイル)。envelope を作るには proto 側で `Envelope` メッセージを仕込むか、Envoy 側で Lua/WASM フィルタを書く必要があり、学習 MVP では過剰。`request_id` 等の横断メタデータは HTTP ヘッダ (Envoy が自動注入する `x-request-id` 等) で運ぶのが自然。

#### 成功レスポンス例

```json
// GET /api/v1/rooms/:id (Room 1 件)
// → chat.v1.GetRoomResponse をそのまま JSON 化
{
  "room": {
    "id": "abc-123",
    "name": "雑談部屋",
    "createdBy": "alice-uuid",
    "memberCount": 3,
    "createdAt": "2026-05-02T10:30:00Z"
  }
}

// GET /api/v1/rooms (自分の参加ルーム一覧)
// → ページネーション情報は next_cursor フィールドが proto に同居 (envelope に分けない)
{
  "rooms": [
    { "id": "abc", "name": "雑談", "createdBy": "alice-uuid", "memberCount": 2, "createdAt": "..." },
    { "id": "def", "name": "ランチ", "createdBy": "bob-uuid", "memberCount": 5, "createdAt": "..." }
  ],
  "nextCursor": ""
}
```

> **JSON フィールド名**: Envoy Transcoder のデフォルトは **camelCase** (proto の `created_at` → JSON の `createdAt`)。snake_case を使いたい場合は Envoy 側で `print_options.preserve_proto_field_names: true` を設定する (Phase 4 で確定)。

#### エラーレスポンス

gRPC のエラーは `google.rpc.Status` の標準 JSON 形式で返る (Transcoder のデフォルト):

```json
// HTTP 404
{
  "code": 5,
  "message": "room: not found",
  "details": []
}
```

`code` は **gRPC code (整数)**、HTTP ステータスは Transcoder が gRPC code から自動マップする。

### HTTP ステータスコード (gRPC code → HTTP の対応)

Transcoder が自動マップする標準対応:

| HTTP | gRPC code (整数 / 名前) | 使用場面 (このプロジェクト) |
|------|------------------------|--------------------------|
| 200 | 0 / OK | 取得・更新成功 |
| 400 | 3 / InvalidArgument | バリデーションエラー (空 content / 壊れた cursor 等) |
| 401 | 16 / Unauthenticated | `x-user-id` 欠落 / JWT 不正 (Envoy が返す) |
| 403 | 7 / PermissionDenied | 非メンバーがメッセージ操作した時 / sender_id 不一致 |
| 404 | 5 / NotFound | ルーム / ユーザー未存在 |
| 409 | 6 / AlreadyExists | 既に Join 済みのルームへの再 Join 等 (本実装は冪等扱い) |
| 412 | 9 / FailedPrecondition | メンバーじゃないルームから退出しようとした等 |
| 500 | 13 / Internal | 想定外のサーバーエラー |

> 201 Created / 204 No Content は使わない。Transcoder のマッピング上、gRPC OK は常に 200 になる。リソース作成系も 200 + body にレスポンス message を返す。

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
  google.protobuf.Timestamp created_at = 5;
  // Phase 2 スコープ: テキストのみ。message_type / media_url / parent_id は追加しない。
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
  // sender_id は realtime-service が x-user-id を信用して詰める。
  // chat-service 側で x-user-id (= 認証済み) との一致を検証してなりすましを防ぐ。
  string sender_id = 2;
  string content = 3;
}

message SendMessageResponse {
  Message message = 1;
}

message GetMessagesRequest {
  string room_id = 1;
  // limit が 0 / 未指定の時はサーバー側でデフォルト (50) を当て、上限 200 で clamp。
  int32 limit = 2;
  // cursor は前回レスポンスの next_cursor をそのまま渡す不透明値 (created_at + id を base64(JSON) でエンコード)。
  string cursor = 3;
}

message GetMessagesResponse {
  // created_at の降順 (新しいもの順)。
  repeated Message messages = 1;
  // 続きがある時のみ非空。空文字なら末尾。has_more 相当はクライアント側で `next_cursor != ""` で判定する。
  string next_cursor = 2;
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

ブラウザから realtime-service に張るリアルタイム通信路。メッセージ送信・受信はすべてこの経路で行う。

### 設計の前提 (Phase 2 スコープ)

- **1 接続 = 1 room**: 接続時に query parameter `room_id` で対象 room を確定する。複数 room を 1 接続で扱う `subscribe` / `unsubscribe` モデルは採用しない。room を切替えたら接続も張り直す
- **テキストメッセージのみ**: 画像 / ファイル / スレッド返信はスコープ外
- **JSON 形式**: フラット型 (`{"type":"...","content":"..."}` のような同階層配置)。type が増えてきたら envelope 型 (`{"type":"...","data":{...}}`) への移行を検討
- **アプリレベルの ping/pong は持たない**: WebSocket 標準の Ping/Pong 制御フレームに任せる (ブラウザのライブラリが自動処理)

### 認証経路 (フロント向け契約)

ブラウザは **必ず Envoy 経由** で realtime-service に接続する。理由は 2 つ:

1. ブラウザの WebSocket API は Upgrade リクエストにカスタムヘッダー (Authorization / `x-user-id` 等) を載せられない (Sec-WebSocket-Protocol しか付けられない仕様) → **JWT は query で渡すしかない**
2. アプリ側は JWT を検証しない設計 (`pkg/auth/context.go` の `RequesterID` は `x-user-id` メタデータを信じて読むだけ) → **検証は Envoy 責務**

```
ブラウザ ──[ ?token=<JWT> ]──▶ Envoy ──[ x-user-id: <user_id> ]──▶ realtime-service
   (TLS, 公開経路)              (JWT 検証)         (内側通信、平文 OK)
```

### フロント向け接続 URL

```
wss://gateway/ws?room_id=<room_id>&token=<JWT>
```

| query | 役割 | 出所 |
|------|------|------|
| `room_id` | 1 接続 = 1 room の対象 room を指定 | クライアント側で決める |
| `token` | アクセストークン (JWT)。Envoy が検証する | `POST /api/v1/auth/login` のレスポンス `access_token` |

Envoy は `token` を検証して JWT の `sub` claim を `x-user-id` ヘッダに詰めて realtime-service にリクエストを流す。realtime-service はヘッダを信用して `user_id` を得る。

### CLI ツール用の補助 (フロント向けではない)

dev で Envoy 抜きでテストする時 (Phase 2 ステップ 8 / Phase 4 の e2e スクリプト等) のみ、`x-user-id` を query で渡せるフォールバックがある:

```
ws://localhost:8081/ws?room_id=<room_id>&x-user-id=<user_id>
```

**ブラウザからは使わない** (この経路はあくまで CLI ツール / 統合テスト用)。dev でフロントを動かしたい場合は Phase 4 の `compose.yaml` で Envoy standalone を立て、`?token=<JWT>` 経路に統一すること。

### クライアント → サーバー メッセージ

```json
// メッセージ送信
{ "type": "message", "content": "Hello!" }
```

`room_id` / `sender_id` は冗長なので含めない:
- `room_id` は接続時の query で確定済み
- `sender_id` は Envoy が注入する `x-user-id` ヘッダから決まる (なりすまし防止のためクライアント指定値は採用しない)

### サーバー → クライアント メッセージ

```json
// 新規メッセージ (自分の echo + 他者発言の両方)
{
  "type": "message",
  "room_id": "room_abc",
  "sender_id": "user_789",
  "content": "Hello!",
  "created_at": "2026-05-02T10:30:00Z"
}

// エラー
{
  "type": "error",
  "code": "invalid_json",
  "message": "json: cannot unmarshal ..."
}
```

エラー `code` 一覧 (Phase 2):

| code | 意味 |
|------|------|
| `invalid_json` | クライアント送信メッセージが JSON としてパースできない |
| `unknown_type` | `type` が `message` 以外で realtime-service が処理できない |

## 関連ドキュメント

- [マイクロサービス詳細](./microservices.md)
- [データモデル設計](./data-model.md)
- [リアルタイムメッセージフロー (永続化 + 配信の 2 経路)](../flow/realtime-message-flow.md)
