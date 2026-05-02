package message

import (
	"context"
	"sort"
	"sync"
)

// InMemRepository は goroutine セーフなインメモリ Repository 実装。
// `go test ./...` を PostgreSQL 無しで走らせるために使う (Phase 1 の room と同じ方針)。
type InMemRepository struct {
	mu       sync.Mutex
	messages []Message
}

func NewInMemRepository() *InMemRepository {
	return &InMemRepository{}
}

func (r *InMemRepository) Insert(_ context.Context, m *Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, *m)
	return nil
}

func (r *InMemRepository) ListByRoom(_ context.Context, roomID string, limit int, cursor Cursor) ([]Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var filtered []Message
	for _, m := range r.messages {
		if m.RoomID != roomID {
			continue
		}
		if !cursor.IsZero() {
			// cursor より新しい / 同位置のものはスキップ (PostgreSQL の (created_at, id) < ... と同じ意味)
			if !lessThan(m.CreatedAt.UnixNano(), m.ID, cursor.CreatedAt.UnixNano(), cursor.ID) {
				continue
			}
		}
		filtered = append(filtered, m)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if !filtered[i].CreatedAt.Equal(filtered[j].CreatedAt) {
			return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
		}
		return filtered[i].ID > filtered[j].ID
	})
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

// lessThan は (createdNs, id) < (cursorNs, cursorID) のタプル比較。
func lessThan(createdNs int64, id string, cursorNs int64, cursorID string) bool {
	if createdNs != cursorNs {
		return createdNs < cursorNs
	}
	return id < cursorID
}
