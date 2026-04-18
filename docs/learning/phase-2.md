# Phase 2: chat-service 追加 + サービス間 gRPC 通信

---

## 学習目標

2 つ目のサービス (chat-service) を **Go で完結** して実装する。user-service は Phase 1 で完成済みなので、2 プロセスを `go run` で並列起動し、**localhost 越しの gRPC 通信** を体験する。

**K8s・Envoy・REST 公開は Phase 4 まで登場しない**。Phase 2 のゴールは「chat-service が gRPC で動き、user-service に `GetUser` を呼べる」まで。

| # | 目標 | 詳細 |
|---|------|------|
| 1 | 2 つ目のサービスを設計・実装できる | chat-service を垂直分割で実装 |
| 2 | サービス間 gRPC 通信を実装できる | chat → user の Unary RPC |
| 3 | Database-per-Service を実践できる | `chatdb` を `userdb` と別 DB に |
| 4 | マイクロサービスの「独立性」を体験できる | 片方再起動してももう片方が動き続ける |
| 5 | ローカルで複数プロセス開発を回せる | Makefile で `make run-user` / `make run-chat` |

---

## 前提知識

- **Phase 1 完了**: user-service が `go run` + `docker postgres` で動作し、`grpcurl` で RPC が叩けること
- gRPC Unary RPC の実装経験 (Phase 1 で体験済み)
- `TrustedUserID` Interceptor の挙動を理解している

---

## 構成 (Phase 2 完了時のローカル環境)

```
[開発者ターミナル]
   │
   ├── go run user-service  → :50051 (gRPC) + :8082 (JWKS)
   │         ↑
   │         │ gRPC (chat → user)
   │         │
   ├── go run chat-service  → :50052 (gRPC)
   │
   ├── docker postgres       → :5432 (userdb / chatdb)
   │
   └── grpcurl (x-user-id を手動注入してテスト)
```

**K8s / Envoy はまだない**。2 プロセスが localhost で通信する最小構成。

---

## ステップ構成

| 部 | テーマ | ステップ |
|----|--------|----------|
| A | chat-service の proto 定義 | 1 |
| B | chat-service の垂直分割実装 | 2〜4 |
| C | サービス間 gRPC 通信 (chat → user) | 5 |
| D | ローカル開発環境の整備 | 6 |
| E | テストと仕上げ | 7 |

---

## A. chat-service の proto 定義

### ステップ 1: proto スキーマ

- [ ] `proto/chat/v1/chat.proto` を新規作成
- [ ] `ChatService` の RPC 一覧:

| カテゴリ | RPC | 説明 |
|---------|-----|------|
| ルーム管理 | `CreateRoom` / `GetRoom` / `ListRooms` / `SearchRooms` | 公開ルーム (作成・自分の一覧・詳細・検索) |
| 参加管理 | `JoinRoom` / `LeaveRoom` | 本人が自己参加・自己退出 (招待・追放なし) |
| メッセージ | `SendMessage` / `GetMessages` | メッセージ作成と履歴取得 (編集・削除はスコープ外) |

- [ ] `google.api.http` アノテーションは **REST 公開する RPC にのみ付ける** (Phase 4 の REST 自動公開で使う)
- [ ] **`SendMessage` には付けない** (クライアントからは WebSocket 経由、内部的には realtime-service が呼び出すだけなので REST 公開不要)

```protobuf
import "google/api/annotations.proto";

service ChatService {
  // REST 公開するもの: google.api.http を付ける
  rpc CreateRoom(CreateRoomRequest) returns (CreateRoomResponse) {
    option (google.api.http) = {post: "/api/v1/rooms", body: "*"};
  }
  rpc ListRooms(ListRoomsRequest) returns (ListRoomsResponse) {
    option (google.api.http) = {get: "/api/v1/rooms"};
  }
  rpc SearchRooms(SearchRoomsRequest) returns (SearchRoomsResponse) {
    option (google.api.http) = {get: "/api/v1/rooms/search"};
  }
  rpc GetRoom(GetRoomRequest) returns (GetRoomResponse) {
    option (google.api.http) = {get: "/api/v1/rooms/{room_id}"};
  }
  rpc JoinRoom(JoinRoomRequest) returns (JoinRoomResponse) {
    option (google.api.http) = {post: "/api/v1/rooms/{room_id}/join", body: "*"};
  }
  rpc LeaveRoom(LeaveRoomRequest) returns (LeaveRoomResponse) {
    option (google.api.http) = {delete: "/api/v1/rooms/{room_id}/members/me"};
  }
  rpc GetMessages(GetMessagesRequest) returns (GetMessagesResponse) {
    option (google.api.http) = {get: "/api/v1/rooms/{room_id}/messages"};
  }

  // REST 公開しないもの: google.api.http を付けない (WebSocket 経由で realtime-svc から呼ばれる)
  rpc SendMessage(SendMessageRequest) returns (SendMessageResponse);
}
```

- [ ] `buf lint` / `buf generate` で `gen/go/chat/v1/` に生成

**確認ポイント**: `chatv1.ChatServiceServer` インターフェースが Go コードとして使える。

---

## B. chat-service の垂直分割実装

### ステップ 2: ディレクトリ骨組みとモジュール

user-service と同様に垂直分割するが、**chat-service は 1 つの gRPC サービス (`ChatService`) に Room と Message の RPC が同居** するため、gRPC トランスポート層は `internal/grpc/` に切り出して両ドメインを束ねる。

```
services/chat-service/
├── cmd/server/main.go
├── go.mod
├── internal/
│   ├── config/
│   ├── room/                     # Room 集約 (rooms + room_members)
│   │   ├── room.go               # エンティティ (Room / RoomMember)
│   │   ├── service.go            # Create/Get/List/Search/Join/Leave/EnsureMember
│   │   ├── repository.go         # interface + PostgreSQL 実装
│   │   └── *_test.go
│   ├── message/                  # Message 集約 (messages)
│   │   ├── message.go            # エンティティ
│   │   ├── service.go            # Send/GetMessages
│   │   ├── repository.go
│   │   └── *_test.go
│   └── grpc/                     # gRPC トランスポート層 (両ドメインを束ねる)
│       └── server.go             # ChatServiceServer を実装、proto↔domain 変換
└── migrations/
    ├── 001_create_rooms.up.sql / down.sql
    ├── 002_create_room_members.up.sql / down.sql
    └── 003_create_messages.up.sql / down.sql
```

### なぜ `grpc/` を別パッケージに切り出すか

user-service は `internal/user/grpc_server.go` を同パッケージ内に置けた (`UserService` の RPC 全部が `user` ドメインに属すため)。

chat-service は違う:
- `ChatService` gRPC は **1 つのインターフェース** に `CreateRoom` / `SendMessage` / `JoinRoom` 等が全部含まれる
- これを `room/grpc_server.go` と `message/grpc_server.go` に分割すると、**gRPC サーバー登録が 2 箇所に割れる** (インターフェース分割不可)
- そこで `internal/grpc/server.go` に一本化し、`room.Service` と `message.Service` に委譲する構造にする

```go
// internal/grpc/server.go
type Server struct {
    chatv1.UnimplementedChatServiceServer
    rooms    *room.Service
    messages *message.Service
}

func (s *Server) CreateRoom(ctx context.Context, req *chatv1.CreateRoomRequest) (*chatv1.CreateRoomResponse, error) {
    r, err := s.rooms.Create(ctx, req.GetName())
    if err != nil { return nil, toGRPCError(err) }
    return &chatv1.CreateRoomResponse{Room: toProto(r)}, nil
}

func (s *Server) SendMessage(ctx context.Context, req *chatv1.SendMessageRequest) (*chatv1.SendMessageResponse, error) {
    senderID, _ := interceptor.UserIDFromContext(ctx)
    // 認可: Room ↔ Message を横断する唯一の箇所
    if err := s.rooms.EnsureMember(ctx, req.GetRoomId(), senderID); err != nil {
        return nil, toGRPCError(err)
    }
    m, err := s.messages.Send(ctx, req.GetRoomId(), senderID, req.GetContent())
    if err != nil { return nil, toGRPCError(err) }
    return &chatv1.SendMessageResponse{Message: toProto(m)}, nil
}
```

**ビジネスロジックは `room/` と `message/` に分割、トランスポート変換と横断認可は `grpc/` に集約** する定番パターン。

- [ ] `services/chat-service/` を `go mod init`
- [ ] `go.work` に `./services/chat-service` を追加

---

### ステップ 3: Database-per-Service (`chatdb`)

user-service とは **別の DB** を使う。同一の PostgreSQL インスタンス内で論理分離する。

```bash
# Phase 1 で起動した docker postgres に chatdb を追加
docker exec -it chat-postgres psql -U chat -d postgres -c "CREATE DATABASE chatdb;"

# chat-service のマイグレーション
migrate -path services/chat-service/migrations \
  -database "postgres://chat:chat@localhost:5432/chatdb?sslmode=disable" up
```

- [ ] `chatdb` に `rooms` / `room_members` / `messages` テーブル作成
- [ ] **`messages.sender_id` は UUID で持つが外部キー制約は張らない** (サービス境界を跨ぐ FK はアンチパターン)

**確認ポイント**: `psql ... chatdb` で 3 テーブルが存在。`userdb` とは論理的に分離されている。

---

### ステップ 4: Room / Message の Go 実装

- [ ] Room ドメイン (`internal/room/`): `CreateRoom` / `GetRoom` / `ListRooms` / `SearchRooms` / `JoinRoom` / `LeaveRoom`
- [ ] Message ドメイン (`internal/message/`): `SendMessage` / `GetMessages`
- [ ] Cursor-based ページネーション (メッセージ履歴用)
- [ ] **`TrustedUserID` Interceptor を Phase 1 の `pkg/interceptor/` から import** (chat-service 側では一切書き直さない)
- [ ] リソース所有者認可 (他人の代わりにメッセージ送信できない等、`sender_id` と requester の一致確認)

```go
func (s *MessageService) SendMessage(ctx context.Context, p SendParams) (*Message, error) {
    requesterID, _ := interceptor.UserIDFromContext(ctx)
    if p.SenderID != requesterID {
        return nil, status.Error(codes.PermissionDenied, "cannot send as another user")
    }
    // 永続化...
}
```

> **スコープ外**: メッセージの編集 (EditMessage) と削除 (DeleteMessage) は Phase 2 のスコープから外した。リアルタイム同期の複雑度 (`message_edited` / `message_deleted` イベントの扱い) が学習主題から外れるため。将来の発展課題として残す。

**確認ポイント**: bufconn で Room の CRUD と Message の Send / Get が通る。

---

## C. サービス間 gRPC 通信 (chat → user)

### ステップ 5: chat-service から user-service を呼ぶ

`GetRoom` の応答で **メンバーの display_name 等を含めるため**、chat-service から user-service の `GetUser` を呼ぶ。これがサービス間 gRPC 通信の体験対象。

- [ ] chat-service に user-service gRPC クライアントを組み込む
- [ ] 起動時に `grpc.Dial` で長寿命接続を確立 (`localhost:50051`)
- [ ] `GetRoom` 時に `room_members` の `user_id` 一覧を取り、**各メンバーの表示情報を user-service から取得**
- [ ] **`x-user-id` を下流の呼び出しにも伝搬**:

```go
func (s *RoomService) GetRoom(ctx context.Context, roomID string) (*Room, error) {
    requesterID, _ := interceptor.UserIDFromContext(ctx)

    room, err := s.repo.GetByID(ctx, roomID)
    if err != nil {
        return nil, err
    }

    // 下流 (user-service) に x-user-id を伝搬
    outCtx := metadata.AppendToOutgoingContext(ctx, "x-user-id", requesterID)

    for i, m := range room.Members {
        u, err := s.users.GetUser(outCtx, &userv1.GetUserRequest{UserId: m.UserID})
        if err != nil {
            // メンバーが見つからない等は表示だけ落とす (致命的エラーにしない)
            continue
        }
        room.Members[i].DisplayName = u.User.DisplayName
    }
    return room, nil
}
```

- [ ] エラーハンドリング: user-service からの `codes.NotFound` をドメインエラーに変換
- [ ] タイムアウト (`context.WithTimeout`)
- [ ] N+1 問題の認識 (今は `GetUser` を N 回呼ぶ。将来 `GetUsers` の bulk API で解決する発展課題)

**確認ポイント**: 2 プロセスを並走 (`go run user-service` と `go run chat-service`) し、`grpcurl` で chat-service の `GetRoom` を叩くと、user-service の `GetUser` が呼ばれてメンバーの display_name が埋まる。

---

## D. ローカル開発環境の整備

### ステップ 6: Makefile の拡充

`make run-user` / `make run-chat` などで開発フローを定型化。

```makefile
# Makefile に追記
.PHONY: db-up db-migrate run-user run-chat

db-up:
	docker run -d --name chat-postgres \
	  -e POSTGRES_USER=chat -e POSTGRES_PASSWORD=chat -e POSTGRES_DB=userdb \
	  -p 5432:5432 postgres:15-alpine
	@sleep 2
	docker exec chat-postgres psql -U chat -d postgres -c "CREATE DATABASE chatdb;"

db-migrate:
	migrate -path services/user-service/migrations \
	  -database "postgres://chat:chat@localhost:5432/userdb?sslmode=disable" up
	migrate -path services/chat-service/migrations \
	  -database "postgres://chat:chat@localhost:5432/chatdb?sslmode=disable" up

run-user:
	DATABASE_URL=postgres://chat:chat@localhost:5432/userdb?sslmode=disable \
	go run ./services/user-service/cmd/server

run-chat:
	DATABASE_URL=postgres://chat:chat@localhost:5432/chatdb?sslmode=disable \
	USER_SERVICE_ADDR=localhost:50051 \
	go run ./services/chat-service/cmd/server
```

**確認ポイント**: ターミナル 2 つで `make run-user` と `make run-chat` を並列起動できる。

---

## E. テストと仕上げ

### ステップ 7: bufconn と統合テスト

- [ ] chat-service 側のユニットテスト (fake user-service client で)
- [ ] bufconn + fake user-service による結合テスト
- [ ] grpcurl スクリプトで end-to-end シナリオ検証

**確認ポイント**: `go test ./...` が全体で PASS。2 プロセス並走での grpcurl シナリオも成功。

---

## 成果物

Phase 2 完了時に以下が動作していること:

- [ ] chat-service が `go run` で起動、`:50052` で gRPC 応答
- [ ] `chatdb` と `userdb` が論理分離されている
- [ ] chat-service → user-service の gRPC 通信で存在確認が動作
- [ ] `x-user-id` が chat-service から user-service まで伝搬
- [ ] リソース所有者認可 (他人になりすましての送信を拒否)
- [ ] 2 プロセス並走での統合シナリオが `grpcurl` で叩ける
- [ ] `go test ./...` が PASS

> **まだ無いもの** (Phase 4 で追加): Envoy Gateway、SecurityPolicy、Transcoder、REST 公開、Dockerfile、K8s マニフェスト。

### ディレクトリ構成 (Phase 2 完了時)

```
go-microservices-chat/
├── pkg/
│   ├── auth/
│   └── interceptor/
├── services/
│   ├── user-service/   # Phase 1 完了
│   └── chat-service/   # Phase 2 で追加
│       ├── cmd/server/main.go
│       ├── internal/
│       │   ├── config/
│       │   ├── room/
│       │   └── message/
│       ├── migrations/
│       └── go.mod
├── proto/
│   ├── user/v1/user.proto
│   └── chat/v1/chat.proto   # Phase 2 で追加
├── gen/go/
│   ├── user/v1/
│   └── chat/v1/
├── Makefile                 # run-user / run-chat 追加
└── go.work
```

### 通信フロー (Phase 2 完了時)

```mermaid
sequenceDiagram
    participant Dev as grpcurl
    participant CS as chat-service (:50052)
    participant US as user-service (:50051)
    participant DB1 as chatdb
    participant DB2 as userdb

    Dev->>CS: GetRoom (x-user-id=alice)
    CS->>CS: TrustedUserID → Context
    CS->>DB1: SELECT rooms + room_members
    loop for each member.user_id
      CS->>US: GetUser (metadata: x-user-id=alice)
      US->>DB2: SELECT users
      US->>CS: GetUserResponse (display_name 等)
    end
    CS->>Dev: GetRoomResponse (メンバー情報を enrich 済み)
```

---

## 学べる技術

| カテゴリ | 技術 | 用途 |
|----------|------|------|
| マイクロサービス | Database-per-Service | サービス独立性 |
| サービス間通信 | gRPC Unary + metadata 伝搬 | `x-user-id` の連鎖 |
| ローカル開発 | 複数プロセス並走 (`go run`) | K8s なしでマイクロサービス体験 |
| 認可 | リソース所有者チェック | chat-service 内でも user-service と同じパターン |

---

## 前のフェーズ

[Phase 1: user-service (Go で完結)](./phase-1.md)

## 次のフェーズ

Phase 2 が完了したら [Phase 3: realtime-service (WebSocket + gRPC Streaming + Redis Pub/Sub)](./phase-3.md) に進む。リアルタイム配信を実装し、3 つ目のサービスを追加する。**K8s・Envoy は Phase 4 まで登場しない**。
