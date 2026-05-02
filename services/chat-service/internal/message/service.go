package message

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// defaultListLimit は GetMessages で limit 未指定時のデフォルト件数。
// Phase 1 の room と同じ理由で 50 / 上限 200 で揃える。
const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// Service は Message ドメインのビジネスロジック。
//
// 認可方針: Service は Room ↔ Message を跨いだ認可 (EnsureMember) を持たず、
// gRPC 層 (message.GRPCAdapter) が room.Service.EnsureMember を呼んでから Send する。
// Service 自身は「sender_id == requester」の同一性チェックのみ行う。
type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// NewServiceWithClock は now() を差し替えた Service を返す (テスト専用)。
// 本番経路は NewService を使う。
func NewServiceWithClock(repo Repository, now func() time.Time) *Service {
	return &Service{repo: repo, now: now}
}

// Send はメッセージを永続化する。引数 senderID は呼び出し元 (gRPC 層) が
// auth.RequesterID(ctx) と SendMessageRequest.SenderID の一致を確認した後の値。
// content の trim と空チェックだけここで行う。
func (s *Service) Send(ctx context.Context, roomID, senderID, content string) (*Message, error) {
	if senderID == "" || roomID == "" {
		return nil, fmt.Errorf("%w: room_id / sender_id required", ErrInvalidArgument)
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("%w: content is required", ErrInvalidArgument)
	}
	m := &Message{
		ID:        uuid.NewString(),
		RoomID:    roomID,
		SenderID:  senderID,
		Content:   content,
		CreatedAt: s.now().UTC(),
	}
	if err := s.repo.Insert(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

// GetMessages は room の履歴を新しい順で返す。pagination は (created_at, id) の不透明 cursor。
// 認可は gRPC 層で済ませる前提 (=「自分が member の room の履歴のみ取れる」を caller が保証)。
func (s *Service) GetMessages(ctx context.Context, roomID string, limit int, cursor string) ([]Message, string, error) {
	if roomID == "" {
		return nil, "", fmt.Errorf("%w: room_id required", ErrInvalidArgument)
	}
	cur, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	limit = effectiveLimit(limit)
	// 続きがあるか判定するために limit+1 件取って末尾を切る方式にする。
	msgs, err := s.repo.ListByRoom(ctx, roomID, limit+1, cur)
	if err != nil {
		return nil, "", err
	}
	var next string
	if len(msgs) > limit {
		// limit 件目の末尾の (created_at, id) が次回の cursor。
		last := msgs[limit-1]
		next = encodeCursor(Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
		msgs = msgs[:limit]
	}
	return msgs, next, nil
}

func effectiveLimit(limit int) int {
	if limit <= 0 {
		return defaultListLimit
	}
	if limit > maxListLimit {
		return maxListLimit
	}
	return limit
}

// Cursor は (created_at, id) のタプル。pagination の境界点。
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

func (c Cursor) IsZero() bool { return c.ID == "" && c.CreatedAt.IsZero() }

// cursor は外向けに見せたくないので不透明エンコード (base64(json))。中身が変わってもクライアント
// にとっては「前回のレスポンスをそのまま返す」だけで使える。
type wireCursor struct {
	C time.Time `json:"c"`
	I string    `json:"i"`
}

func encodeCursor(c Cursor) string {
	if c.IsZero() {
		return ""
	}
	b, _ := json.Marshal(wireCursor{C: c.CreatedAt, I: c.ID})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	var w wireCursor
	if err := json.Unmarshal(raw, &w); err != nil {
		return Cursor{}, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	if w.I == "" {
		return Cursor{}, fmt.Errorf("%w: empty id", ErrInvalidCursor)
	}
	return Cursor{CreatedAt: w.C, ID: w.I}, nil
}
