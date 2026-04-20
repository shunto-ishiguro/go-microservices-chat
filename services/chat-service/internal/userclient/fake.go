package userclient

import (
	"context"
	"errors"
	"sync"
)

// Fake はテスト用のインメモリ Client。シナリオ実行前に Set で Profile を仕込む。
//
// 使い方:
//
//	fake := userclient.NewFake()
//	fake.Set(&userclient.Profile{ID: "alice", DisplayName: "Alice", AvatarURL: "..."})
//	// → GetUser("alice") や BatchGetUsers(["alice"]) で profile が返る
//	// → 未登録の ID は GetUser では not found、BatchGetUsers では結果から欠落 (部分成功)
type Fake struct {
	mu       sync.Mutex
	profiles map[string]*Profile
}

func NewFake() *Fake {
	return &Fake{profiles: map[string]*Profile{}}
}

func (f *Fake) Set(p *Profile) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.profiles[p.ID] = p
}

func (f *Fake) GetUser(_ context.Context, userID string) (*Profile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.profiles[userID]
	if !ok {
		return nil, errors.New("userclient.fake: not found")
	}
	return p, nil
}

func (f *Fake) BatchGetUsers(_ context.Context, userIDs []string) ([]Profile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Profile, 0, len(userIDs))
	for _, id := range userIDs {
		if p, ok := f.profiles[id]; ok {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (f *Fake) Close() error { return nil }
