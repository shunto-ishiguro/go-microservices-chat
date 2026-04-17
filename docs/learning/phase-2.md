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
| ルーム管理 | `CreateRoom` / `GetRoom` / `ListRooms` | 1:1・グループ |
| メンバー | `AddMember` / `RemoveMember` | ルームへの出入り |
| メッセージ | `SendMessage` / `GetMessages` / `EditMessage` / `DeleteMessage` | メッセージ CRUD |
| 既読 | `MarkAsRead` | 既読マーカー |

- [ ] `google.api.http` アノテーションを書いておく (Phase 4 の REST 自動公開で使う)

```protobuf
import "google/api/annotations.proto";

service ChatService {
  rpc CreateRoom(CreateRoomRequest) returns (CreateRoomResponse) {
    option (google.api.http) = {
      post: "/api/v1/rooms"
      body: "*"
    };
  }
  rpc SendMessage(SendMessageRequest) returns (SendMessageResponse) {
    option (google.api.http) = {
      post: "/api/v1/rooms/{room_id}/messages"
      body: "*"
    };
  }
  // ...
}
```

- [ ] `buf lint` / `buf generate` で `gen/go/chat/v1/` に生成

**確認ポイント**: `chatv1.ChatServiceServer` インターフェースが Go コードとして使える。

---

## B. chat-service の垂直分割実装

### ステップ 2: ディレクトリ骨組みとモジュール

user-service と同じパターン。

```
services/chat-service/
├── cmd/server/main.go
├── go.mod
├── internal/
│   ├── config/
│   ├── room/
│   │   ├── room.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   └── grpc_server.go
│   └── message/
│       ├── message.go
│       ├── service.go
│       ├── repository.go
│       └── grpc_server.go
└── migrations/
    ├── 001_create_rooms.up.sql / down.sql
    ├── 002_create_room_members.up.sql / down.sql
    └── 003_create_messages.up.sql / down.sql
```

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

- [ ] Room ドメイン (`internal/room/`): `CreateRoom` / `GetRoom` / `ListRooms` / `AddMember` / `RemoveMember`
- [ ] Message ドメイン (`internal/message/`): `SendMessage` / `GetMessages` / `EditMessage` / `DeleteMessage` / `MarkAsRead`
- [ ] Cursor-based ページネーション (メッセージ履歴用)
- [ ] **`TrustedUserID` Interceptor を Phase 1 の `pkg/interceptor/` から import** (chat-service 側では一切書き直さない)
- [ ] リソース所有者認可 (他人のメッセージを編集・削除できない)

```go
func (s *MessageService) EditMessage(ctx context.Context, messageID string, content string) error {
    requesterID, _ := interceptor.UserIDFromContext(ctx)
    msg, err := s.repo.GetByID(ctx, messageID)
    if err != nil {
        return err
    }
    if msg.SenderID != requesterID {
        return status.Error(codes.PermissionDenied, "cannot edit other's message")
    }
    // ...
}
```

**確認ポイント**: bufconn で Room / Message の CRUD シナリオが通る。

---

## C. サービス間 gRPC 通信 (chat → user)

### ステップ 5: chat-service から user-service を呼ぶ

chat-service から user-service の `GetUser` を呼び、ルームメンバー / メッセージ送信者の存在確認をする。

- [ ] chat-service に user-service gRPC クライアントを組み込む
- [ ] 起動時に `grpc.Dial` で長寿命接続を確立 (`localhost:50051`)
- [ ] `CreateRoom` 時にメンバー ID の存在を `GetUser` で確認
- [ ] `SendMessage` 時に送信者 ID の存在を確認
- [ ] **`x-user-id` を下流の呼び出しにも伝搬**:

```go
func (s *RoomService) CreateRoom(ctx context.Context, p CreateRoomParams) (*Room, error) {
    requesterID, _ := interceptor.UserIDFromContext(ctx)

    // 下流 (user-service) に x-user-id を伝搬
    outCtx := metadata.AppendToOutgoingContext(ctx, "x-user-id", requesterID)

    for _, uid := range p.MemberIDs {
        _, err := s.users.GetUser(outCtx, &userv1.GetUserRequest{UserId: uid})
        if err != nil {
            return nil, fmt.Errorf("invalid member %s: %w", uid, err)
        }
    }
    // ルーム作成
}
```

- [ ] エラーハンドリング: user-service からの `codes.NotFound` をドメインエラーに変換
- [ ] タイムアウト (`context.WithTimeout`)

**確認ポイント**: 2 プロセスを並走 (`go run user-service` と `go run chat-service`) し、`grpcurl` で chat-service の `CreateRoom` を叩くと、user-service の `GetUser` が呼ばれ、存在しない user_id で失敗する。

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
- [ ] リソース所有者認可 (他人のメッセージ編集を拒否)
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

    Dev->>CS: CreateRoom (x-user-id=alice)
    CS->>CS: TrustedUserID → Context
    loop for each member_id
      CS->>US: GetUser (metadata: x-user-id=alice)
      US->>DB2: SELECT users
      US->>CS: GetUserResponse
    end
    CS->>DB1: INSERT rooms
    CS->>Dev: CreateRoomResponse
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
