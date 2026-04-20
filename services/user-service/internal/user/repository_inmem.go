package user

import (
	"context"
	"sync"
	"time"
)

// InMemRepository は goroutine セーフなインメモリ Repository 実装。
// `go test ./...` を PostgreSQL なしで走らせるために用意している。
type InMemRepository struct {
	mu     sync.Mutex
	users  map[string]User
	tokens map[string]RefreshToken
}

func NewInMemRepository() *InMemRepository {
	return &InMemRepository{
		users:  map[string]User{},
		tokens: map[string]RefreshToken{},
	}
}

// CreateUser は PostgreSQL 実装の `UNIQUE(email)` / `UNIQUE(username)` 制約を
// 線形探索でエミュレートする。テスト規模 (数十ユーザー) で十分な性能。
func (r *InMemRepository) CreateUser(_ context.Context, u *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.users {
		if existing.Email == u.Email || existing.Username == u.Username {
			return ErrAlreadyExists
		}
	}
	r.users[u.ID] = *u
	return nil
}

func (r *InMemRepository) GetUserByID(_ context.Context, id string) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &u, nil
}

func (r *InMemRepository) GetUsersByIDs(_ context.Context, ids []string) ([]User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []User
	for _, id := range ids {
		if u, ok := r.users[id]; ok {
			out = append(out, u)
		}
	}
	return out, nil
}

func (r *InMemRepository) GetUserByEmail(_ context.Context, email string) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.users {
		if u.Email == email {
			return &u, nil
		}
	}
	return nil, ErrNotFound
}

func (r *InMemRepository) UpdateUser(_ context.Context, u *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.users[u.ID]
	if !ok {
		return ErrNotFound
	}
	existing.DisplayName = u.DisplayName
	existing.AvatarURL = u.AvatarURL
	existing.StatusText = u.StatusText
	existing.UpdatedAt = u.UpdatedAt
	r.users[u.ID] = existing
	return nil
}

func (r *InMemRepository) CreateRefreshToken(_ context.Context, t *RefreshToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[t.ID] = *t
	return nil
}

func (r *InMemRepository) GetRefreshTokenByHash(_ context.Context, hash string) (*RefreshToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tokens {
		if t.TokenHash == hash {
			tc := t
			return &tc, nil
		}
	}
	return nil, ErrNotFound
}

func (r *InMemRepository) RevokeRefreshToken(_ context.Context, id string, revokedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tokens[id]
	if !ok {
		return ErrNotFound
	}
	t.RevokedAt = &revokedAt
	r.tokens[id] = t
	return nil
}
