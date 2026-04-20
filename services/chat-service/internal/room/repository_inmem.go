package room

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// InMemRepository は goroutine セーフなインメモリ Repository 実装。
// `go test ./...` を PostgreSQL なしで走らせるために用意している。
//
// データ構造:
//
//	rooms   : room_id → Room
//	members : room_id → (user_id → joined_at)  ※ ネストした map でメンバーシップと参加時刻を管理
type InMemRepository struct {
	mu      sync.Mutex
	rooms   map[string]Room
	members map[string]map[string]time.Time
}

func NewInMemRepository() *InMemRepository {
	return &InMemRepository{
		rooms:   map[string]Room{},
		members: map[string]map[string]time.Time{},
	}
}

func (r *InMemRepository) CreateRoom(_ context.Context, room *Room) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rooms[room.ID] = *room
	r.members[room.ID] = map[string]time.Time{}
	return nil
}

func (r *InMemRepository) GetRoom(_ context.Context, id string) (*Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &room, nil
}

func (r *InMemRepository) ListRoomsByMember(_ context.Context, userID string, limit int, _ string) ([]Room, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Room
	for roomID, ms := range r.members {
		if _, ok := ms[userID]; ok {
			out = append(out, r.rooms[roomID])
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return applyLimit(out, limit), "", nil
}

// SearchRooms は PostgreSQL の ILIKE に相当する大文字小文字無視の部分一致を
// Go の strings.Contains で模倣する。テスト用途の小規模データに特化した単純実装。
func (r *InMemRepository) SearchRooms(_ context.Context, query string, limit int, _ string) ([]Room, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	query = strings.ToLower(query)
	var out []Room
	for _, room := range r.rooms {
		if strings.Contains(strings.ToLower(room.Name), query) {
			out = append(out, room)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return applyLimit(out, limit), "", nil
}

func (r *InMemRepository) AddMember(_ context.Context, roomID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rooms[roomID]; !ok {
		return ErrNotFound
	}
	if _, ok := r.members[roomID][userID]; ok {
		return ErrAlreadyMember
	}
	r.members[roomID][userID] = time.Now().UTC()
	return nil
}

func (r *InMemRepository) RemoveMember(_ context.Context, roomID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rooms[roomID]; !ok {
		return ErrNotFound
	}
	if _, ok := r.members[roomID][userID]; !ok {
		return ErrNotMember
	}
	delete(r.members[roomID], userID)
	return nil
}

func (r *InMemRepository) ListMembers(_ context.Context, roomID string) ([]Member, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ms, ok := r.members[roomID]
	if !ok {
		return nil, ErrNotFound
	}
	out := make([]Member, 0, len(ms))
	for uid, joined := range ms {
		out = append(out, Member{UserID: uid, JoinedAt: joined})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].JoinedAt.Before(out[j].JoinedAt) })
	return out, nil
}

func (r *InMemRepository) IsMember(_ context.Context, roomID, userID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ms, ok := r.members[roomID]
	if !ok {
		return false, nil
	}
	_, ok = ms[userID]
	return ok, nil
}

func (r *InMemRepository) CountMembers(_ context.Context, roomID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.members[roomID]), nil
}

// applyLimit は limit 以下に切り詰める。limit <= 0 は「無制限」扱い。
func applyLimit(rooms []Room, limit int) []Room {
	if limit <= 0 || limit >= len(rooms) {
		return rooms
	}
	return rooms[:limit]
}
