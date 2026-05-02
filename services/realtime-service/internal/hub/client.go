package hub

// Client は 1 つの WebSocket 接続を表す。
// 「読み取り側 goroutine」と「書き込み側 goroutine」のペアで動かすことを想定し、
// Hub からは send channel に payload を投げる窓口だけが見える。
//
// 実際の WebSocket I/O は ws パッケージで行う (本パッケージは I/O ライブラリ非依存)。
type Client struct {
	UserID string
	RoomID string
	// send は Hub → Client への配信チャネル。書き込み側 goroutine がここから読んで WS に流す。
	// 容量はバッファ。詰まったら Hub.Run の non-blocking send で drop される。
	send chan []byte
}

func NewClient(userID, roomID string, sendBuf int) *Client {
	if sendBuf <= 0 {
		sendBuf = 32
	}
	return &Client{
		UserID: userID,
		RoomID: roomID,
		send:   make(chan []byte, sendBuf),
	}
}

// Outbound はハンドラ側 (書き込み goroutine) が消費する読み取り専用チャネルを返す。
// channel 自体は Hub が close するので、receiver は range で抜ける設計。
func (c *Client) Outbound() <-chan []byte { return c.send }
