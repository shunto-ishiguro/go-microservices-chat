package chatclient

import (
	"context"
	"sync"
)

// Fake は WS handler のユニットテスト用。Sent に呼び出し履歴を保持するだけのスタブ。
type Fake struct {
	mu   sync.Mutex
	Sent []FakeCall
	// Err を非 nil にすると次回以降の SendMessage がそのエラーを返す。
	Err error
}

type FakeCall struct {
	SenderID string
	RoomID   string
	Content  string
}

func NewFake() *Fake { return &Fake{} }

func (f *Fake) SendMessage(_ context.Context, senderID, roomID, content string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.Sent = append(f.Sent, FakeCall{SenderID: senderID, RoomID: roomID, Content: content})
	return nil
}

func (f *Fake) Close() error { return nil }

// Calls は呼び出し履歴のスナップショットをコピーで返す。
func (f *Fake) Calls() []FakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]FakeCall, len(f.Sent))
	copy(out, f.Sent)
	return out
}
