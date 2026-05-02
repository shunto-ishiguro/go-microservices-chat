// Package ws は WebSocket 接続の HTTP Upgrade と JSON フレームのやりとりを担当する。
//
// JSON プロトコル (Phase 2 スコープ):
//
//	クライアント → サーバー:
//	  {"type":"message", "content":"hello"}     // 接続中の room へ送信
//	サーバー → クライアント:
//	  {"type":"message", "id":"...", "room_id":"...", "sender_id":"...",
//	                     "content":"...", "created_at":"RFC3339"}
//	  {"type":"error", "code":"...", "message":"..."}
//
// 「join」型は今のところ持たない: WS 接続時に query parameter `?room_id=...` で 1 接続 = 1 room を
// 確定する設計。将来的に複数 room を 1 接続で扱いたくなったら type:"join" を追加する。
package ws

import "time"

// Inbound はクライアントから届く WS メッセージの最大公約数。
type Inbound struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
}

// Outbound はサーバーがクライアントに送り返す共通形式。
type Outbound struct {
	Type      string    `json:"type"`
	ID        string    `json:"id,omitempty"`
	RoomID    string    `json:"room_id,omitempty"`
	SenderID  string    `json:"sender_id,omitempty"`
	Content   string    `json:"content,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	Code      string    `json:"code,omitempty"`
	Message   string    `json:"message,omitempty"`
}

// Inbound type 値。
const (
	TypeMessage = "message"
	TypeError   = "error"
)
