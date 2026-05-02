package pubsub_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"go-microservices-chat/services/realtime-service/internal/pubsub"
)

func TestInMem_PublishedEventsReachSubscriber(t *testing.T) {
	p := pubsub.NewInMem()
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		mu     sync.Mutex
		events []pubsub.RoomEvent
		done   = make(chan struct{})
	)
	go func() {
		_ = p.Subscribe(ctx, func(ev pubsub.RoomEvent) {
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
			if len(events) == 2 {
				close(done)
			}
		})
	}()

	if err := p.Publish(ctx, pubsub.RoomEvent{RoomID: "r1", Payload: []byte("hello")}); err != nil {
		t.Fatal(err)
	}
	if err := p.Publish(ctx, pubsub.RoomEvent{RoomID: "r1", Payload: []byte("world")}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("subscriber did not receive both events in time")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 2 || string(events[0].Payload) != "hello" || string(events[1].Payload) != "world" {
		t.Errorf("unexpected events: %+v", events)
	}
}

func TestInMem_SubscribeReturnsOnContextCancel(t *testing.T) {
	p := pubsub.NewInMem()
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- p.Subscribe(ctx, func(pubsub.RoomEvent) {}) }()

	cancel()
	select {
	case err := <-errCh:
		if err == nil {
			t.Errorf("expected non-nil error on cancel")
		}
	case <-time.After(time.Second):
		t.Fatalf("Subscribe did not return on cancel")
	}
}
