# リアルタイムメッセージ配信フロー (Phase 2 realtime-service)

Phase 2 で組む「Alice が送ったメッセージが Bob の画面にリアルタイムで届く」までの流れを、**どのプロセスの何行目が動くか** まで追いかけて解説する。

**Redis Pub/Sub を最初から配信バスとして使う** 設計。realtime-service は **プロセス (あるいは Pod) を複数並べても Go コードを変更しなくて済む** よう、最初から Pub/Sub 前提で実装する。本リポジトリ Phase 4 の compose で `--scale realtime-service=2` にしても、infra リポジトリ側の K8s で `replicas: 2` にしても、app 側のコードは一切変わらない。

---

## 登場人物

| 役割 | 説明 |
|------|------|
| Alice | ブラウザで `#general` ルームを開いている送信者 |
| Bob   | 別のブラウザで同じ `#general` ルームを開いている受信者 |
| user-service    | ユーザー情報・認証 (:50051) |
| chat-service    | メッセージの永続化 (:50052) |
| realtime-service| WebSocket 終端 + 配信 (:8081) |
| Redis           | 配信バス (Pub/Sub) (:6379) |

---

## シチュエーション

> **Alice が「こんにちは」と送信 → Bob の画面にリアルタイムで表示される**

この 1 つの体験の裏で、プロセス間・Redis を介して何が起きるかを追う。

---

## 事前状態: 2 ユーザーがルームを開いた時点

Alice も Bob も、ブラウザから realtime-service へ **WebSocket 接続** を張っている。

```
Alice のブラウザ ──WS常時接続──→ realtime-service
Bob   のブラウザ ──WS常時接続──→ realtime-service
```

realtime-service は Hub (プロセス内の Go 構造体) の中で「`#general` ルームには Alice と Bob の 2 つの WebSocket が所属している」と記憶している。

---

## realtime-service 起動時にやっていること (重要)

realtime-service は **起動した瞬間に** Redis に対して `PSUBSCRIBE room:*` を張りっぱなしにする。

```go
// services/realtime-service/internal/pubsub/redis.go (抜粋)
func (p *Redis) Subscribe(ctx context.Context, onMessage func(RoomEvent)) error {
    ps := p.rdb.PSubscribe(ctx, "room:*")
    defer ps.Close()
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case msg, ok := <-ps.Channel():     // ← ここで Redis からの push を待機
            if !ok {
                return fmt.Errorf("subscription closed")
            }
            roomID := strings.TrimPrefix(msg.Channel, "room:")
            onMessage(RoomEvent{RoomID: roomID, Payload: []byte(msg.Payload)})
        }
    }
}
```

この時点では何も流れていない。**Redis に誰かが PUBLISH するのを待っている状態**。

```
realtime-service ══ Redis SUBSCRIBE (アイドル) ══ Redis
```

これが realtime-service 実装の肝になる通信路。以降の ④〜⑤ で実際にイベントが流れる。

---

## 時系列: Alice が送信ボタンを押してから Bob に届くまで

### ① Alice のブラウザ → realtime-service (WebSocket)

Alice が送信ボタンを押す。ブラウザの JavaScript が WebSocket にメッセージを書き込む。
WS 接続時の URL `?room_id=general` で送信先 room は確定済みなので、フレームには `room_id` を含めない。

```json
{"type": "message", "content": "こんにちは"}
```

```
Alice ──WS──→ realtime-service
```

---

### ② realtime-service が「保存」と「配信」を並行実行

realtime-service の WebSocket ハンドラが受信し、**2 つの処理を並行** で走らせる。

```go
// services/realtime-service/internal/ws/handler.go (抜粋)
func (h *Handler) HandleMessage(ctx context.Context, userID, roomID, content string) {
    out := Outbound{
        Type: TypeMessage, RoomID: roomID, SenderID: userID, Content: content,
        CreatedAt: h.now().UTC(),
    }
    payload, _ := json.Marshal(out)

    // (a) 永続化: chatclient 経由で chat-service に gRPC SendMessage (内部で x-user-id を outgoing metadata に詰める)
    go func() {
        persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        if err := h.chatClient.SendMessage(persistCtx, userID, roomID, content); err != nil {
            h.logger.Error("ws: persist failed", "error", err)
        }
    }()

    // (b) 配信: Redis に PUBLISH (RoomEvent でラップ)
    go func() {
        pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        if err := h.pubsub.Publish(pubCtx, pubsub.RoomEvent{RoomID: roomID, Payload: payload}); err != nil {
            h.logger.Error("ws: publish failed", "error", err)
        }
    }()
}
```

```
              ┌───gRPC Unary SendMessage───→ chat-service ──→ PostgreSQL
realtime-svc ─┤
              └───Redis PUBLISH room:general─→ Redis
```

**ポイント**: 永続化と配信を分離。配信は chat-service の保存完了を待たない = 低遅延。

---

### ③ chat-service が DB 保存

gRPC Unary の保存経路。これは永続化だけの責務。

```go
// services/chat-service/internal/message/grpc.go
// (実際は requester == sender_id チェックと EnsureMember 認可を挟むが、
//  ここでは「永続化が責務」という骨格だけ示す)
func (a *GRPCAdapter) SendMessage(ctx context.Context, req *chatv1.SendMessageRequest) (*chatv1.SendMessageResponse, error) {
    m, err := a.svc.Send(ctx, req.GetRoomId(), req.GetSenderId(), req.GetContent())  // PostgreSQL に永続化
    if err != nil {
        return nil, mapError(err)
    }
    return &chatv1.SendMessageResponse{Message: toProto(m)}, nil
}
```

chat-service は **リアルタイム配信に一切関与しない**。純粋な「保存するサービス」。

---

### ④ Redis が全 SUBSCRIBE 購読者に配る

Redis は `PUBLISH room:general <payload>` を受け取ると、**`room:*` を SUBSCRIBE している全クライアント** に配信する。

```
         ┌──push──→ realtime-svc (自分自身)
Redis ───┤
         ├──push──→ realtime-svc-2 (別プロセス / 別レプリカにも届く)
         └──push──→ realtime-svc-3 (...)
```

**1 インスタンスだけでも自分自身で SUBSCRIBE しているので届く**。複数プロセス / 複数レプリカに増やしても同じコードのまま横連携が効く。

---

### ⑤ realtime-service が Redis から受信

起動時に張ってあった `PSubscribe` goroutine がここで起きる。

```go
// 起動時に張った Subscriber goroutine の中身 (実装は redis.go の Subscribe ループ + main の onMessage コールバック)
case msg, ok := <-ps.Channel():
    // msg.Channel = "room:general"
    // msg.Payload = '{"type":"message","room_id":"general","sender_id":"alice","content":"...","created_at":"..."}'
    roomID := strings.TrimPrefix(msg.Channel, "room:")
    h.LocalBroadcast(hub.LocalEvent{RoomID: roomID, Payload: []byte(msg.Payload)})
```

---

### ⑥ Hub がルーム内の全 WebSocket にブロードキャスト

```go
// services/realtime-service/internal/hub/hub.go (broadcast case 抜粋)
case ev := <-h.broadcast:
    for c := range h.rooms[ev.RoomID] {   // 同プロセスに繋いでいる Alice or Bob (どちらか片方が居る)
        // バッファ満杯の slow client は drop して Hub 全体は止めない
        select {
        case c.send <- ev.Payload:
        default:
        }
    }
```

---

### ⑦ realtime-service → Alice / Bob のブラウザ (WebSocket)

各クライアントの書き込み goroutine が、`client.send` から取り出して `conn.WriteMessage()` でブラウザに送る。

```json
{"type": "message",
 "room_id": "general",
 "sender_id": "alice-uuid",
 "content": "こんにちは",
 "created_at": "2026-05-02T10:30:00Z"}
```

Bob のブラウザの JavaScript が `ws.onmessage` でこれを拾い、画面に「Alice: こんにちは」を表示する。**これでゴール**。

---

## 全体図 (時系列)

```
[①] Alice ──WS──→ realtime-service
                      │
[②]         ┌─────────┴─────────┐
            ▼ (並行)              ▼ (並行)
       gRPC Unary              Redis PUBLISH
       SendMessage              room:general
            │                       │
            ▼                       ▼
[③]  chat-service              [Redis]
        ↓                           │
   PostgreSQL INSERT                │
                                    │ (SUBSCRIBE 済みの全員に配る)
[④]                                 ▼
[⑤] realtime-service ←──push───── [Redis]
        │
[⑥]     └─→ Hub が #general ルームの全 WebSocket を特定
[⑦]           │
              ├──WS──→ Alice のブラウザ (自分の送信が反映)
              └──WS──→ Bob のブラウザ (リアルタイム受信) ★ゴール★
```

---

## 各通信路の役割整理

| # | 区間 | プロトコル | 用途 |
|---|------|-----------|------|
| ① | Alice → realtime | WebSocket (送信) | ユーザー入力の受信 |
| ②a | realtime → chat | gRPC Unary | 1 件保存依頼 |
| ②b | realtime → Redis | Redis PUBLISH | 配信バスに投函 |
| ③ | chat → PostgreSQL | SQL INSERT | 永続化 |
| ④ | Redis → realtime | Redis SUBSCRIBE (push) | 配信バスから取り出し |
| ⑤ | realtime 内部 | Go channel | Hub への流し込み |
| ⑥ | Hub → 各 Client | Go channel | ルーム内ファンアウト |
| ⑦ | realtime → Bob | WebSocket (配信) | ブラウザ表示 |

---

## 補足: なぜ永続化と配信を分けるのか

### 疑問

> chat-service が保存してから Redis に Publish すれば 1 経路で済むのでは？

### 理由 1: 遅延を最小化したい

チャットは「保存より先に画面に出る」のが許される性質。**配信を保存完了に待たせる理由がない**。ユーザー体験の観点では 100ms でも早い方が良い。

### 理由 2: 責務を明確に分離したい

- `chat-service` = **永続化のサービス** (検索・履歴取得・監査)
- `Redis` = **配信のバス** (リアルタイム fan-out)

役割が違うので経路も分ける。どちらが落ちても部分的に動く (保存だけ / 配信だけ) という耐障害性にもつながる。

### 理由 3: chat-service の依存方向を一方向に保つ

chat-service は **realtime-service の存在を知らなくて済む**。Redis を中継にすることで、chat-service → realtime-service の直接依存が無くなる。

---

## 補足: Redis Pub/Sub とは何か (郵便受けの例え)

④〜⑤ で出てくる Redis Pub/Sub がピンと来ない人向けに、手前から説明する。

### そもそも「何を解決したいのか」

realtime-service が **1 プロセス内では Hub (Go channel) で WebSocket 間の配信ができる**。でも複数プロセスや複数レプリカに増やすと、こうなる:

```
Alice は realtime-svc-1 に WebSocket 接続している
Bob   は realtime-svc-2 に WebSocket 接続している
```

Alice が送信したメッセージは **realtime-svc-1 のプロセス内 Hub にしか届かない**。realtime-svc-2 にいる Bob には届かない。

**プロセスを跨いだ配信手段** が必要 → それが Redis Pub/Sub。

---

### 郵便受けで例える

realtime-service のインスタンス達を「家」、ブラウザを「住人」、**Redis を「街の中央郵便局」** だと思う。

```
┌── realtime-svc-1 ──┐                            ┌── realtime-svc-2 ──┐
│   (Alice の家)     │                            │   (Bob の家)       │
│                    │                            │                    │
│   ブラウザ Alice   │                            │   ブラウザ Bob     │
│      │             │                            │        ▲           │
│      │ WS          │                            │        │ WS        │
│      ▼             │                            │        │           │
│   Hub              │                            │     Hub            │
│      │             │                            │        ▲           │
│      │ PUBLISH     │                            │        │ push      │
│      ▼             │                            │        │           │
└─────┼──────────────┘                            └────────┼───────────┘
      │                                                    │
      ▼                                                    │
   ┌──────────────────────────────────────────────────────────┐
   │                  📮 Redis (中央郵便局)                  │
   │  room:general ──PUBLISH──→ 全 SUBSCRIBE 者に一斉配信    │
   └──────────────────────────────────────────────────────────┘
```

### 流れ

1. Alice のメッセージを受けた realtime-svc-1 が **郵便局 (Redis) に投函** (`PUBLISH room:general`)
2. 郵便局は **`room:*` を SUBSCRIBE している全員の家にコピーを配る**
3. Bob の家 (realtime-svc-2) が受け取り → Bob の WebSocket に書き込み

`PUBLISH` が 1 回でも、SUBSCRIBE 者の数だけ **自動的に fan-out** される。Redis の Pub/Sub はこれが標準動作。

---

### Go channel との対比

| 項目 | Go channel (Hub) | Redis Pub/Sub |
|------|------------------|---------------|
| 繋ぐもの | 同じプロセス内の goroutine | **別プロセス (複数 Pod) をまたいで配る** |
| 実装 | Go の言語機能 | 外部ミドルウェア |
| 受信者数 | 1 メッセージ = 1 受信者 | 1 メッセージ = SUBSCRIBE 全員 |
| 用途 | プロセス内の WebSocket 管理 | プロセス間の fan-out |

「Go channel は家の中の内線、Redis Pub/Sub は街の中央郵便局」くらいのイメージで OK。

---

## プロセスを複数に増やすとどう化けるか

realtime-service は **1 プロセスだけで動かしていても**、Redis 経由で publish/subscribe する構造を最初から組んである。その副作用:

**docker-compose で `--scale realtime-service=2`、あるいは K8s で `replicas: 2` にするだけで** (どちらも infra 側の宣言を変えるだけで app のコードは無変更):

- Alice はプロセス A に接続、Bob はプロセス B に接続
- Alice の投稿はプロセス A から Redis に PUBLISH
- Redis がプロセス A とプロセス B の両方に push
- プロセス B の Hub が Bob に WebSocket で配信

**app 側の Go コードは一切変更しない**。これが realtime-service 実装で最初から Redis を使う最大の理由。

### 手元での検証 (infra repo を立てずに)

Redis を docker で 1 個立て、`go run ./services/realtime-service/cmd/server` を **ポートを変えて 2 プロセス** 起動すれば、同じ挙動が `PORT=8081` と `PORT=8082` のプロセス間で確認できる (詳細は [Phase 2 ステップ 8](../phase/phase-2.md))。

---

## まとめ

| 要素 | 役割 |
|------|------|
| WebSocket | ブラウザと realtime-service の双方向通信 |
| Hub (Go channel) | 1 プロセス内の WebSocket を束ねる |
| gRPC Unary (SendMessage) | chat-service に永続化を依頼 (配信には関与しない) |
| Redis Pub/Sub | **プロセス (Pod) を跨いだ配信バス** |
| 永続化と配信の並行 | 遅延最小化 + 責務分離 |

chat-service は **永続化専任**。配信は **realtime-service + Redis** のコンビで完結する。realtime-service を複数 Pod に増やしても同じコードのまま動く。

---

## 参考

- [Phase 2 ドキュメント](../phase/phase-2.md)
- [API Design](../architecture/api-design.md)
- [Microservices 概要](../architecture/microservices.md)
