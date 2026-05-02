package pubsub

import (
	"context"
	"sync"
)

// InMem は同一プロセス Go channel を使った PubSub 実装。Redis 無しでテストする用。
//
// 本番経路 (Redis) と同じ "Publish した瞬間に全 Subscriber に届く" セマンティクスを再現する。
// 1 プロセスに複数の Subscriber が居る前提はないので、購読者は 0 or 1 で十分:
// realtime-service は起動時に 1 度だけ Subscribe を呼び、ws ハンドラから Publish するだけ。
type InMem struct {
	mu      sync.Mutex
	ch      chan RoomEvent
	closed  bool
	closeCh chan struct{}
}

func NewInMem() *InMem {
	return &InMem{
		ch:      make(chan RoomEvent, 64),
		closeCh: make(chan struct{}),
	}
}

func (p *InMem) Publish(_ context.Context, ev RoomEvent) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()
	select {
	case p.ch <- ev:
	case <-p.closeCh:
	}
	return nil
}

func (p *InMem) Subscribe(ctx context.Context, onMessage func(RoomEvent)) error {
	for {
		select {
		case ev, ok := <-p.ch:
			if !ok {
				return nil
			}
			onMessage(ev)
		case <-ctx.Done():
			return ctx.Err()
		case <-p.closeCh:
			return nil
		}
	}
}

// Close は Publish / Subscribe 双方を解放する。テストで goroutine リークを防ぐため。
func (p *InMem) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	close(p.closeCh)
}
