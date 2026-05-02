package message_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go-microservices-chat/services/chat-service/internal/message"
)

// newServiceWithClock は now() を固定して、(created_at, id) の cursor 比較を
// 決定的にテストするためのヘルパー。Send が呼ばれるたびに ticks をインクリメントする。
func newServiceWithClock(t *testing.T) (*message.Service, *message.InMemRepository, func(time.Time)) {
	t.Helper()
	repo := message.NewInMemRepository()
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	svc := message.NewServiceWithClock(repo, func() time.Time { return now })
	set := func(t time.Time) { now = t }
	return svc, repo, set
}

func TestService_Send_PersistsMessage(t *testing.T) {
	svc := message.NewService(message.NewInMemRepository())
	m, err := svc.Send(context.Background(), "room-1", "alice", "hi")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if m.RoomID != "room-1" || m.SenderID != "alice" || m.Content != "hi" {
		t.Errorf("unexpected message: %+v", m)
	}
	if m.ID == "" || m.CreatedAt.IsZero() {
		t.Errorf("id/created_at not populated: %+v", m)
	}
}

func TestService_Send_RejectsEmptyContent(t *testing.T) {
	svc := message.NewService(message.NewInMemRepository())
	if _, err := svc.Send(context.Background(), "room-1", "alice", "   "); !errors.Is(err, message.ErrInvalidArgument) {
		t.Errorf("err = %v, want ErrInvalidArgument", err)
	}
}

func TestService_Send_RejectsMissingIDs(t *testing.T) {
	svc := message.NewService(message.NewInMemRepository())
	if _, err := svc.Send(context.Background(), "", "alice", "hi"); !errors.Is(err, message.ErrInvalidArgument) {
		t.Errorf("missing room: err = %v", err)
	}
	if _, err := svc.Send(context.Background(), "room-1", "", "hi"); !errors.Is(err, message.ErrInvalidArgument) {
		t.Errorf("missing sender: err = %v", err)
	}
}

func TestService_GetMessages_NewestFirstAndPaginates(t *testing.T) {
	svc, _, setNow := newServiceWithClock(t)

	// 3 件投入: t0, t1, t2 の順。期待出力は t2, t1, t0 (新しい順)。
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	for i, ts := range []time.Time{t0, t0.Add(time.Second), t0.Add(2 * time.Second)} {
		setNow(ts)
		if _, err := svc.Send(context.Background(), "room-1", "alice", string('a'+rune(i))); err != nil {
			t.Fatal(err)
		}
	}

	// 1 ページ目: limit=2 → 続きあり (next_cursor 非空)
	page1, next, err := svc.GetMessages(context.Background(), "room-1", 2, "")
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d", len(page1))
	}
	if !page1[0].CreatedAt.After(page1[1].CreatedAt) {
		t.Errorf("not in DESC order: %+v", page1)
	}
	if next == "" {
		t.Errorf("next_cursor should be non-empty when more pages remain")
	}

	// 2 ページ目: 続きを取って最後の 1 件 + 末尾
	page2, next2, err := svc.GetMessages(context.Background(), "room-1", 2, next)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2 len = %d, want 1", len(page2))
	}
	if next2 != "" {
		t.Errorf("next_cursor should be empty at the end, got %q", next2)
	}
}

func TestService_GetMessages_OtherRoomIsolated(t *testing.T) {
	svc := message.NewService(message.NewInMemRepository())
	if _, err := svc.Send(context.Background(), "room-1", "alice", "in 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(context.Background(), "room-2", "alice", "in 2"); err != nil {
		t.Fatal(err)
	}
	got, _, err := svc.GetMessages(context.Background(), "room-1", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Content != "in 1" {
		t.Errorf("room isolation broken: %+v", got)
	}
}

func TestService_GetMessages_RejectsBrokenCursor(t *testing.T) {
	svc := message.NewService(message.NewInMemRepository())
	_, _, err := svc.GetMessages(context.Background(), "room-1", 0, "!!not-base64!!")
	if !errors.Is(err, message.ErrInvalidCursor) {
		t.Errorf("err = %v, want ErrInvalidCursor", err)
	}
}
