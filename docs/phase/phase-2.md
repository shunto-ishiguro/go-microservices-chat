# Phase 2: chat (Message) + realtime-service (WebSocket + Redis Pub/Sub)

---

## ディレクトリ構成 (Phase 2 完了時)

```
go-microservices-chat/
├── proto/
│   ├── user/v1/user.proto              # Phase 1 から
│   └── chat/v1/chat.proto              # ★ SendMessage / GetMessages 追加
├── gen/go/                             # buf generate で再生成
├── pkg/                                # Phase 1 から (変更なし)
├── services/
│   ├── user-service/                   # Phase 1 完了 (変更なし)
│   ├── chat-service/
│   │   ├── internal/
│   │   │   ├── room/                   # Phase 1 から (grpc.go から UnimplementedChatServiceServer の embed を外す)
│   │   │   ├── message/                # ★ Phase 2 で新規
│   │   │   │   ├── message.go          # Message エンティティ + ドメインエラー
│   │   │   │   ├── service.go          # Send / GetMessages (cursor-based)
│   │   │   │   ├── repository.go       # Repository interface + PostgreSQL 実装
│   │   │   │   ├── repository_inmem.go
│   │   │   │   ├── grpc.go             # message.GRPCAdapter (SendMessage / GetMessages)
│   │   │   │   └── *_test.go           # service_test.go (InMem) + grpc_test.go (bufconn)
│   │   │   └── grpc/                   # ★ Phase 2 で新規 (合流層)
│   │   │       └── server.go           # Server: room.GRPCAdapter + message.GRPCAdapter を named field で保持し forward
│   │   ├── migrations/
│   │   │   └── 003_create_messages.up.sql / down.sql  # ★ 追加
│   │   └── ...
│   └── realtime-service/               # ★ Phase 2 で新規
│       ├── cmd/server/main.go          # WebSocket :8081 + Redis 接続 + Hub 起動 + chat-svc クライアント
│       ├── internal/
│       │   ├── config/config.go        # REDIS_ADDR / CHAT_SERVICE_ADDR / HTTP_ADDR を env から読む
│       │   ├── hub/
│       │   │   ├── hub.go              # Register / Unregister / LocalBroadcast (goroutine + channel)
│       │   │   ├── client.go           # 1 接続 = 1 Client (read/write goroutine)
│       │   │   └── *_test.go
│       │   ├── pubsub/
│       │   │   ├── pubsub.go           # Publisher / Subscriber interface + RoomEvent
│       │   │   ├── redis.go            # Redis 実装: PUBLISH / PSUBSCRIBE "room:*"
│       │   │   ├── inmem.go            # InMem 実装 (Go channel、Redis 不要でテスト用)
│       │   │   └── inmem_test.go       # InMem 実装で go test PASS (Redis 実装は infra repo or Phase 4 compose で疎通)
│       │   ├── chatclient/
│       │   │   ├── client.go           # chat-svc.SendMessage で永続化依頼
│       │   │   └── fake.go             # テスト用 fake (Calls() で履歴記録)
│       │   └── ws/
│       │       ├── handler.go          # HTTP Upgrade → Client 登録 → 読み取りループ → chat-svc + PUBLISH
│       │       ├── handler_test.go     # httptest + coder/websocket Dial で end-to-end
│       │       └── protocol.go         # WS メッセージ型 (Inbound: type=message のみ / Outbound: type=message or error)
│       └── go.mod
└── Makefile                            # run-realtime 追加
```

> Dockerfile は Phase 3 でまとめて作成する。K8s / Envoy ルート / Rate Limit / Transcoder 等は **infra リポジトリ側の責務** なのでここには出てこない。

---

## スコープ

Phase 1 の基盤に乗せる形で (a) chat-service に Message 機能、(b) realtime-service を丸ごと (WebSocket + Hub + Redis Pub/Sub) を実装する。**Pub/Sub を最初から採用** するのがポイント: 単一プロセスでも Pub/Sub を通す設計にしておけば、infra 側で複数インスタンスに増やした時にコードを 1 行も変えずに fan-out が成立する。

**前提**: Phase 1 完了 (user-service + chat-service の Room 部分が動く)。

---

## ステップ構成

| 部 | テーマ | ステップ |
|----|--------|----------|
| A | chat-service の Message 機能 | 1〜3 |
| B | realtime-service 実装 | 4〜7 |
| C | 2 プロセス Pub/Sub 検証 | 8 |

---

## A. chat-service の Message 機能

### ステップ 1: proto 拡張 + コード再生成

- [ ] `proto/chat/v1/chat.proto` に `SendMessage` / `GetMessages` を追加
- [ ] `SendMessage` は `google.api.http` を付けない (realtime-svc から gRPC でしか呼ばれない、REST 公開しない)
- [ ] `GetMessages` は `google.api.http` で GET `/api/v1/rooms/{room_id}/messages`
- [ ] `buf generate`

**確認ポイント**: `gen/go/chat/v1/` が再生成される。

---

### ステップ 2: Message ドメイン

- [ ] `internal/message/message.go` + `service.go` + `repository.go` + `repository_inmem.go`
- [ ] **`auth.RequesterID(ctx)` と `SendMessageRequest.SenderID` の一致確認は GRPCAdapter 側 (ステップ 3) で行う**。Service 自身は受け取った senderID を信用して INSERT する責務分担
- [ ] `Service.Send`: 引数で受け取った `(roomID, senderID, content)` でドメイン Message を生成して Repository に INSERT
- [ ] `Service.GetMessages`: cursor-based pagination (`(created_at, id)` のタプルを base64(JSON) でエンコード、limit+1 件 fetch して末尾を切るスタイルで `next_cursor` を返す)
- [ ] `migrations/003_create_messages.up.sql` (FK は張らない、`sender_id` は UUID で保持。INDEX は `(room_id, created_at DESC, id DESC)` で cursor pagination を高速化)

**確認ポイント**: `service_test.go` で InMem 実装上の `Send` / `GetMessages` (新着順 / pagination / cursor 不正) がテーブル駆動テストで PASS。

---

### ステップ 3: Message GRPCAdapter 新設 + `internal/grpc/` 合流層

Phase 1 では `room.GRPCAdapter` が `ChatServiceServer` を単独で満たしていたが、Phase 2 で Message RPC が加わるので構造を変える:

1. `internal/message/grpc.go` に `message.GRPCAdapter` を新設 (SendMessage / GetMessages)
2. `room/grpc.go` から `UnimplementedChatServiceServer` embed を外す (合流層に移す)
3. `internal/grpc/server.go` を新設し、両アダプタを **named field で保持して各 RPC を forward** する `Server` を置く

```go
// internal/message/grpc.go
package message

type GRPCAdapter struct {
    svc   *Service
    rooms *room.Service  // EnsureMember (横断認可) のため
}

func (a *GRPCAdapter) SendMessage(ctx context.Context, req *chatv1.SendMessageRequest) (*chatv1.SendMessageResponse, error) {
    senderID, ok := auth.RequesterID(ctx)
    if !ok {
        return nil, status.Error(codes.Unauthenticated, "missing x-user-id")
    }
    // 認可: Room ↔ Message を横断する唯一の箇所
    if err := a.rooms.EnsureMember(ctx, req.GetRoomId(), senderID); err != nil {
        return nil, mapError(err)
    }
    m, err := a.svc.Send(ctx, req.GetRoomId(), senderID, req.GetContent())
    if err != nil { return nil, mapError(err) }
    return &chatv1.SendMessageResponse{Message: toProto(m)}, nil
}

// internal/grpc/server.go
package grpc

// 注意: 両アダプタを struct embedding する案 (*room.GRPCAdapter + *message.GRPCAdapter を
// フィールド名なしで並べる) は **コンパイルできない**。
// Go は埋め込みフィールド名を「型の simple name」 (パッケージ名を除いた最後の識別子) に決めるため、
// 両方とも `GRPCAdapter` という同名フィールドになり duplicate field エラーになる。
// → named field で保持し、各 RPC メソッドを明示的に forward する形にする。
type Server struct {
    chatv1.UnimplementedChatServiceServer  // forward-compat (将来 RPC 追加時の defaults) はここだけに置く
    rooms    *room.GRPCAdapter
    messages *message.GRPCAdapter
}

func (s *Server) CreateRoom(ctx context.Context, req *chatv1.CreateRoomRequest) (*chatv1.CreateRoomResponse, error) {
    return s.rooms.CreateRoom(ctx, req)
}
// ... GetRoom / ListRooms / SearchRooms / JoinRoom / LeaveRoom / ListRoomMembers も同じパターンで forward

func (s *Server) SendMessage(ctx context.Context, req *chatv1.SendMessageRequest) (*chatv1.SendMessageResponse, error) {
    return s.messages.SendMessage(ctx, req)
}
func (s *Server) GetMessages(ctx context.Context, req *chatv1.GetMessagesRequest) (*chatv1.GetMessagesResponse, error) {
    return s.messages.GetMessages(ctx, req)
}
```

- [ ] `room.GRPCAdapter` から `chatv1.UnimplementedChatServiceServer` embed を外す
- [ ] `internal/message/grpc.go` に `message.GRPCAdapter` を実装 (SendMessage / GetMessages + mapError)
- [ ] `internal/grpc/server.go` で named field 保持 + 9 RPC の forward を書く
- [ ] `cmd/server/main.go` で両アダプタを `chatgrpc.NewServer(...)` に渡して `RegisterChatServiceServer` する

**確認ポイント**: bufconn で `CreateRoom` → `JoinRoom` → `SendMessage` → `GetMessages` が通る。`x-user-id` 未注入や非メンバーで `Unauthenticated` / `PermissionDenied`。

---

## B. realtime-service 実装

### ステップ 4: 骨組み + Hub

- [ ] `services/realtime-service/` を `go mod init`
- [ ] `go.work` に `./services/realtime-service` を追加
- [ ] `internal/hub/hub.go`: Register / Unregister / LocalBroadcast を select で回す 1 goroutine
- [ ] `internal/hub/client.go`: 読み取り / 書き込み goroutine ペア

```go
// internal/hub/hub.go (実装の骨格、エラー処理省略の擬似コード)
func (h *Hub) Run() {
    for {
        select {
        case c := <-h.register:
            if h.rooms[c.RoomID] == nil {
                h.rooms[c.RoomID] = map[*Client]bool{}
            }
            h.rooms[c.RoomID][c] = true
        case c := <-h.unregister:
            // 同じ Client を 2 回 close しないよう先にメンバー確認
            if subs, ok := h.rooms[c.RoomID]; ok {
                if _, member := subs[c]; member {
                    delete(subs, c)
                    close(c.send)
                }
            }
        case ev := <-h.broadcast:
            for c := range h.rooms[ev.RoomID] {
                // バッファ満杯の slow client は drop して Hub 全体を止めない
                select {
                case c.send <- ev.Payload:
                default:
                }
            }
        case <-h.stop:
            return
        }
    }
}
```

**確認ポイント**: `hub_test.go` の以下 3 シナリオが PASS:
- 同 room の購読者全員に payload が届く / 別 room の購読者には届かない
- `Unregister` で send channel が close される (range で抜けられる)
- バッファ満杯の Client があっても Hub が止まらず他 Client には届く (drop 動作)

---

### ステップ 5: Pub/Sub (interface + Redis + InMem)

- [ ] `internal/pubsub/pubsub.go`: `Publisher` / `Subscriber` interface
- [ ] `internal/pubsub/redis.go`: `go-redis` で `PUBLISH` / `PSUBSCRIBE "room:*"`
- [ ] `internal/pubsub/inmem.go`: Go channel ベース (Redis 無しで Hub のテストが書ける)
- [ ] Subscriber が受信イベントを Hub の broadcast に流す

```go
// internal/pubsub/pubsub.go (interface)
type RoomEvent struct {
    RoomID  string
    Payload []byte
}

type Publisher interface {
    Publish(ctx context.Context, ev RoomEvent) error
}

type Subscriber interface {
    // ctx Done か致命エラーまでブロック。main 側で別 goroutine 起動。
    Subscribe(ctx context.Context, onMessage func(RoomEvent)) error
}

// internal/pubsub/redis.go (本番実装)
func (p *Redis) Publish(ctx context.Context, ev RoomEvent) error {
    return p.rdb.Publish(ctx, "room:"+ev.RoomID, ev.Payload).Err()
}

func (p *Redis) Subscribe(ctx context.Context, onMessage func(RoomEvent)) error {
    ps := p.rdb.PSubscribe(ctx, "room:*")
    defer ps.Close()
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case msg, ok := <-ps.Channel():
            if !ok {
                return fmt.Errorf("pubsub.redis: subscription closed")
            }
            roomID := strings.TrimPrefix(msg.Channel, "room:")
            onMessage(RoomEvent{RoomID: roomID, Payload: []byte(msg.Payload)})
        }
    }
}
```

**確認ポイント**: `inmem_test.go` が PASS (`go test ./...` が Redis 無しで通る)。Redis 実装は本リポジトリ Phase 4 の compose、または手動で `docker run redis:7-alpine` を立てて Phase 2 ステップ 8 のクロスプロセス検証で疎通させる。

---

### ステップ 6: WebSocket ハンドラ + chat-svc クライアント

- [ ] `internal/ws/protocol.go`: `Inbound` (Type, Content) / `Outbound` (Type, ID, RoomID, SenderID, Content, CreatedAt, Code, Message) / 定数 `TypeMessage = "message"` / `TypeError = "error"`。**`join` 型は持たない** (1 接続 = 1 room、URL クエリで確定するため)
- [ ] `internal/ws/handler.go`: HTTP Upgrade → Hub に Client 登録 → 読み取りループ。`?room_id=` query 必須、`x-user-id` ヘッダ (Envoy 注入) or query (CLI 補助) で認証ユーザー解決
- [ ] `internal/chatclient/client.go`: `grpc.NewClient(os.Getenv("CHAT_SERVICE_ADDR"))` で長寿命接続。`SendMessage(ctx, senderID, roomID, content)` のシンプルな wrapper にして proto 型は外に漏らさない
- [ ] `internal/chatclient/fake.go`: 呼び出し履歴を `Calls()` で取り出せるテスト用 fake
- [ ] WS 受信時に **goroutine で並行実行**: (a) chat-svc.SendMessage / (b) Redis PUBLISH

```go
// internal/ws/handler.go (実装の骨格)
func (h *Handler) HandleMessage(ctx context.Context, userID, roomID, content string) {
    out := Outbound{
        Type: TypeMessage, RoomID: roomID, SenderID: userID, Content: content,
        CreatedAt: h.now().UTC(),
    }
    payload, _ := json.Marshal(out)

    // (a) 永続化 (fire-and-forget): chatclient が x-user-id を outgoing metadata に詰める
    go func() {
        persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        if err := h.chatClient.SendMessage(persistCtx, userID, roomID, content); err != nil {
            h.logger.Error("ws: persist failed", "error", err)
        }
    }()

    // (b) 配信 (Redis PUBLISH): 同プロセスの Subscriber 経由で自分自身にも echo される
    go func() {
        pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        if err := h.pubsub.Publish(pubCtx, pubsub.RoomEvent{RoomID: roomID, Payload: payload}); err != nil {
            h.logger.Error("ws: publish failed", "error", err)
        }
    }()
}
```

**確認ポイント**: `handler_test.go` で httptest + coder/websocket Dial の end-to-end (受信メッセージが永続化呼び出し + Pub/Sub Publish の両方に流れる / Subscriber 経由で接続中の WS クライアントに届く / `x-user-id` 欠落で 401 / `room_id` 欠落で 400) が PASS。**アプリ側は JWT 検証しない** — WS Upgrade 時の `x-user-id` ヘッダを読むだけ (実環境では Envoy が注入する)。

---

### ステップ 7: `cmd/server/main.go`

- [ ] WebSocket :8081 起動 (`http.ListenAndServe`)
- [ ] Redis 接続 + Hub 起動 + Subscriber 起動
- [ ] SIGTERM で graceful shutdown (活きている WebSocket を close してから Redis 接続を閉じる順序)
- [ ] 環境変数: `REDIS_ADDR` / `CHAT_SERVICE_ADDR`

**確認ポイント**: `go run` でプロセスが起動、SIGTERM で graceful に落ちる。

---

## C. 2 プロセス Pub/Sub 検証 (infra repo が無くても手元で試す最短手順)

### ステップ 8: Redis 1 個立てて realtime 2 プロセスで検証

Redis と 2 つの realtime-service プロセスを手元で立てて、**プロセス境界を跨いで Pub/Sub が機能する** ことを確認する。

**chat-service は起動しなくて良い**: 永続化呼び出し (`chatclient.SendMessage`) は handler 内で fire-and-forget な goroutine で走るので、失敗しても WS 配信経路には影響しない。Step 8 の検証目的 = 「**プロセス間配信**」だけなので chat-service は不要。`CHAT_SERVICE_ADDR` には dummy 値を入れておけば起動できる。

ターミナルを 5 つ用意する: ①Redis ②realtime A ③realtime B ④wscat (Alice) ⑤wscat (Bob)。

```bash
# 事前: wscat を入れる (1 回だけ。お好みで websocat でも可)
npm install -g wscat

# ① Redis (--rm 付きなので Ctrl+C でコンテナごと消える)
docker run --rm -p 6379:6379 --name chat-redis redis:7-alpine

# ② realtime-service プロセス A (HTTP_ADDR=:8081)
REDIS_ADDR=localhost:6379 CHAT_SERVICE_ADDR=localhost:50052 \
HTTP_ADDR=:8081 \
go run ./services/realtime-service/cmd/server

# ③ realtime-service プロセス B (ポートだけずらす)
REDIS_ADDR=localhost:6379 CHAT_SERVICE_ADDR=localhost:50052 \
HTTP_ADDR=:8082 \
go run ./services/realtime-service/cmd/server

# ④ Alice の WS (プロセス A 経由)
#   room_id query は必須 (ハンドラが 1 接続 = 1 room の前提なので、未指定だと 400)。
#   x-user-id ヘッダは Envoy 注入の代わりに dev 用に直接付ける。
wscat -c "ws://localhost:8081/ws?room_id=room-1" -H "x-user-id: alice-uuid"

# ⑤ Bob の WS (プロセス B 経由)
wscat -c "ws://localhost:8082/ws?room_id=room-1" -H "x-user-id: bob-uuid"
```

**送信して検証**: Alice 側 (ターミナル ④) のプロンプト `>` で以下を貼って Enter。

```json
{"type":"message","content":"hi from alice"}
```

**確認ポイント**: Alice 自身 (`④`) と Bob (`⑤`) の **両方** に同じ payload が届く。Bob は別プロセスに繋がっているので、これが「**Redis Pub/Sub で複数インスタンス跨ぎの fan-out が成立している**」一番重要な検証。

```
< {"type":"message","room_id":"room-1","sender_id":"alice-uuid","content":"hi from alice","created_at":"..."}
```

Redis に流れている PUBLISH を直接覗きたければ、別ターミナルで `docker exec -it chat-redis redis-cli MONITOR`。

> これ以上の E2E (ゲートウェイ経由で JWT 付き / 本物の DB / 全コンポーネント連携) は **本リポジトリの Phase 4 (`compose.yaml` + `envoy.yaml`)** で組む。本番向け K8s / NetworkPolicy / Helm 等は infra リポジトリ側の責務。

---

## 成果物

- [ ] `go test ./...` が **Redis / 他プロセス無しで** PASS (InMem pubsub + fake chatclient で完結)
- [ ] chat-service で `SendMessage` / `GetMessages` が bufconn で動く
- [ ] realtime-service が `go run` で WebSocket :8081 を受け付ける
- [ ] Redis + 2 プロセスの手元検証で、**プロセス跨ぎの配信が Redis Pub/Sub 経由で機能する** (ステップ 8)

### メッセージ送信処理のフロー (Phase 2 完了時のイメージ)

**Alice が realtime-svc プロセス A に接続、Bob が realtime-svc プロセス B に接続している状況での送信フロー**:

```mermaid
flowchart TB
    Start["Alice: WebSocket でメッセージ送信<br/>{type: message, room_id, content}"]
    Start --> WS["realtime-svc プロセス A<br/>WS handler が受信"]

    WS --> Fork{"goroutine 2 本で並行実行"}

    Fork -->|永続化経路| P1["chatclient.SendMessage<br/>(gRPC Unary)"]
    P1 --> CS["chat-service (別プロセス)"]
    CS --> DB[("PostgreSQL chatdb<br/>messages テーブル")]

    Fork -->|配信経路| P2["pubsub.Publish(RoomEvent)<br/>PUBLISH room:&lt;room_id&gt;"]
    P2 --> R[("Redis")]

    R -->|PSUBSCRIBE room:*| SA["プロセス A: Subscriber"]
    R -->|PSUBSCRIBE room:*| SB["プロセス B: Subscriber"]

    SA --> HA["プロセス A: Hub<br/>→ 同プロセスに繋いでいる<br/>WS クライアント (Alice 自身) に配信"]
    SB --> HB["プロセス B: Hub<br/>→ 同プロセスに繋いでいる<br/>WS クライアント (Bob) に配信"]

    classDef app fill:#e3f2fd,stroke:#1976d2
    classDef bus fill:#fff3e0,stroke:#f57c00
    classDef store fill:#f3e5f5,stroke:#7b1fa2

    class Start,WS,Fork,P1,P2,SA,SB,HA,HB,CS app
    class R bus
    class DB store
```

**このフローの肝**:

- **永続化と配信を goroutine で並行実行** → Alice の送信遅延を最小化 (chat-service の書き込み完了を待たずに Bob に届く)
- **PUBLISH 先は Redis 1 点集中** → プロセス数が 1 でも N でも PUBLISH する側のコードは同じ
- **Subscriber は各プロセスで独立** → 新しいプロセスを立ち上げた瞬間から自動で配信が届く (Go コード変更不要)
- **chat-service は配信に関与しない** → 永続化専任。リアルタイム性の責務から完全に分離

> この「最初から Redis 通し」の設計で、infra 側でプロセスを 1 → 2 → N に増やしても **app のコードは 1 行も変わらない**。これが Phase 2 で Redis Pub/Sub を素直に入れる最大の理由。

---

## 前のフェーズ / 次のフェーズ

- 前: [Phase 1: user-service + chat-service (Room)](./phase-1.md)
- 次: [Phase 3: Dockerfile + イメージビルド](./phase-3.md)
