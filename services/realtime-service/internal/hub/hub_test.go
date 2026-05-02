package hub_test

import (
	"testing"
	"time"

	"go-microservices-chat/services/realtime-service/internal/hub"
)

// waitFor は channel に値が届くか timeout を待つ簡易ヘルパー。テストを deadlock させない用。
func waitFor(t *testing.T, ch <-chan []byte, want string) {
	t.Helper()
	select {
	case got, ok := <-ch:
		if !ok {
			t.Fatalf("channel closed, want %q", want)
		}
		if string(got) != want {
			t.Fatalf("got %q, want %q", string(got), want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for %q", want)
	}
}

func TestHub_BroadcastReachesRoomMembers(t *testing.T) {
	h := hub.NewHub()
	go h.Run()
	defer h.Stop()

	alice := hub.NewClient("alice", "room-1", 8)
	bob := hub.NewClient("bob", "room-1", 8)
	carol := hub.NewClient("carol", "room-2", 8) // 別 room: 受け取らないはず

	h.Register(alice)
	h.Register(bob)
	h.Register(carol)
	// register が drain されるのを少し待つ (channel 経由の非同期登録)
	time.Sleep(20 * time.Millisecond)

	h.LocalBroadcast(hub.LocalEvent{RoomID: "room-1", Payload: []byte(`{"type":"message"}`)})

	waitFor(t, alice.Outbound(), `{"type":"message"}`)
	waitFor(t, bob.Outbound(), `{"type":"message"}`)

	select {
	case got := <-carol.Outbound():
		t.Fatalf("carol (other room) received %q", string(got))
	case <-time.After(50 * time.Millisecond):
		// 期待通り受け取らない
	}
}

func TestHub_UnregisterClosesChannel(t *testing.T) {
	h := hub.NewHub()
	go h.Run()
	defer h.Stop()

	c := hub.NewClient("alice", "room-1", 8)
	h.Register(c)
	time.Sleep(10 * time.Millisecond)
	h.Unregister(c)

	// Unregister 後は send channel が close される (range で抜けられる)
	select {
	case _, ok := <-c.Outbound():
		if ok {
			t.Fatalf("expected channel to be closed")
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for channel close")
	}
}

func TestHub_DropsWhenClientBufferFull(t *testing.T) {
	// バッファ 1 の Client にバースト送信 → Hub が止まらず、後続 Broadcast が他 Client に届くこと。
	h := hub.NewHub()
	go h.Run()
	defer h.Stop()

	slow := hub.NewClient("slow", "room-1", 1)
	fast := hub.NewClient("fast", "room-1", 8)
	h.Register(slow)
	h.Register(fast)
	time.Sleep(20 * time.Millisecond)

	for i := 0; i < 5; i++ {
		h.LocalBroadcast(hub.LocalEvent{RoomID: "room-1", Payload: []byte("p")})
	}
	// fast には少なくとも 1 通来る
	waitFor(t, fast.Outbound(), "p")
}
