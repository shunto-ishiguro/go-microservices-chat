// Package hub は単一プロセス内の WebSocket クライアント集合を管理する。
//
// Hub は 1 プロセスに 1 つだけ走り、Register / Unregister / LocalBroadcast を
// それぞれ buffered channel で受け取って 1 goroutine で直列処理する (= データ競合なし)。
// ロックを持たない設計なのは、書き込み経路が select 1 箇所に絞られるため map 操作が
// 必ずシリアライズされるから。
//
// 「Local」が付いている命令は同一プロセスに繋いでいる Client にだけ届ける、という意味。
// プロセス境界を跨ぐ配信は pubsub レイヤーの責務 (Redis Pub/Sub)。Hub は受け取った
// payload を機械的に各 Client.send channel に流すだけで、誰がそれを発火したかは知らない。
package hub

import (
	"sync"
)

// LocalEvent は Hub に対する「この room の購読者全員に payload を流して」という命令。
// payload は JSON エンコード済みのバイト列 (WS フレームにそのまま書ける形)。
type LocalEvent struct {
	RoomID  string
	Payload []byte
}

// Hub は WebSocket 接続の登録 / 解除 / ブロードキャストを 1 goroutine で直列化する。
type Hub struct {
	register   chan *Client
	unregister chan *Client
	broadcast  chan LocalEvent

	// rooms[roomID] は購読者集合。set 代わりの bool map で素朴に表現。
	rooms map[string]map[*Client]bool

	// closeOnce は Stop の二重呼びを安全にするため。
	closeOnce sync.Once
	stop      chan struct{}
}

func NewHub() *Hub {
	return &Hub{
		register:   make(chan *Client, 16),
		unregister: make(chan *Client, 16),
		broadcast:  make(chan LocalEvent, 256),
		rooms:      map[string]map[*Client]bool{},
		stop:       make(chan struct{}),
	}
}

// Register は新規接続の Client を hub に追加するキュー操作。
// 同期的な参加完了を待たない (= 後段の goroutine で取り出されてから rooms map に入る)。
func (h *Hub) Register(c *Client) { h.register <- c }

// Unregister は接続終了時に Client を hub から外す。close(c.send) は Run 側で行う。
func (h *Hub) Unregister(c *Client) { h.unregister <- c }

// LocalBroadcast は同プロセスの購読者に payload を配る。
// pubsub.Subscriber が受信イベントを Hub に流す経路もこれを使う。
func (h *Hub) LocalBroadcast(ev LocalEvent) { h.broadcast <- ev }

// Run は Hub のメインループ。プロセスの寿命と同じ 1 goroutine で動かす想定。
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			if h.rooms[c.RoomID] == nil {
				h.rooms[c.RoomID] = map[*Client]bool{}
			}
			h.rooms[c.RoomID][c] = true
		case c := <-h.unregister:
			if subs, ok := h.rooms[c.RoomID]; ok {
				if _, member := subs[c]; member {
					delete(subs, c)
					close(c.send)
					if len(subs) == 0 {
						delete(h.rooms, c.RoomID)
					}
				}
			}
		case ev := <-h.broadcast:
			for c := range h.rooms[ev.RoomID] {
				// 受信側 goroutine が遅い時は send channel が満杯になる。
				// Hub 全体を止めないよう non-blocking で流す: 詰まった client は drop。
				// (本格的なバックプレッシャは Phase 4 以降で再検討、今は Hub の生存を優先)
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

// Stop は Run ループを終了させる。プロセス終了時に main から呼ぶ。
func (h *Hub) Stop() {
	h.closeOnce.Do(func() { close(h.stop) })
}
