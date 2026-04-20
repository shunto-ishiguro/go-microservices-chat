package room

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"go-microservices-chat/pkg/auth"
)

// defaultListLimit: 一覧系 RPC (ListMyRooms / SearchRooms) で limit が未指定の時のデフォルト件数。
// 上限は effectiveLimit の 200 で頭打ちにする (過大な limit で DB / ネットワークを食わないため)。
const defaultListLimit = 50

// Service はルーム管理のビジネスロジックを持つ: 作成・一覧・Join/Leave と、
// 他所 (Phase 2 の Message RPC など) から使われる EnsureMember 認可述語を提供する。
type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// ============================================================
// External API: gRPC 経由で Web クライアントから呼ばれる
// ============================================================
//
// 全 RPC が Envoy ゲートウェイ経由で、x-user-id をそのまま信じて処理する。
// 認可 (自分以外のルーム参照等) は Service 内で x-user-id を基に判定。

// CreateRoom は呼び出し元を作成者として公開ルームを作成する。
// 作成者は自動的にメンバーとして追加される。
func (s *Service) CreateRoom(ctx context.Context, name string) (*Room, error) {
	creatorID, ok := auth.RequesterID(ctx)
	if !ok {
		return nil, fmt.Errorf("%w: missing requester", ErrInvalidArgument)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidArgument)
	}
	room := &Room{
		ID:        uuid.NewString(),
		Name:      name,
		CreatedBy: creatorID,
		CreatedAt: s.now().UTC(),
	}
	if err := s.repo.CreateRoom(ctx, room); err != nil {
		return nil, err
	}
	if err := s.repo.AddMember(ctx, room.ID, creatorID); err != nil {
		return nil, err
	}
	return room, nil
}

// GetRoom はルームの軽量情報 (ヘッダ用) とメンバー数を返す。
// メンバーの配列は返さない — 画面 #9 の ListRoomMembers で別途取る。
func (s *Service) GetRoom(ctx context.Context, id string) (*Room, int, error) {
	r, err := s.repo.GetRoom(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	count, err := s.repo.CountMembers(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	return r, count, nil
}

// ListRoomMembers は指定ルームのメンバー一覧を返す (enrich 前のドメイン型)。
// gRPC 層で userclient.BatchGetUsers を呼んで display_name / avatar_url を差し込む。
func (s *Service) ListRoomMembers(ctx context.Context, roomID string) ([]Member, error) {
	if _, err := s.repo.GetRoom(ctx, roomID); err != nil {
		return nil, err
	}
	return s.repo.ListMembers(ctx, roomID)
}

// ListMyRooms は呼び出し元が参加しているルーム一覧を返す (画面 #5)。
// 「自分以外のルーム」を返す経路は無いので、参照範囲が x-user-id で閉じる。
func (s *Service) ListMyRooms(ctx context.Context, limit int, cursor string) ([]Room, string, error) {
	userID, ok := auth.RequesterID(ctx)
	if !ok {
		return nil, "", fmt.Errorf("%w: missing requester", ErrInvalidArgument)
	}
	return s.repo.ListRoomsByMember(ctx, userID, effectiveLimit(limit), cursor)
}

// SearchRooms は公開ルームの名前部分一致検索 (画面 #3)。全ルームが public なので認可は無い。
func (s *Service) SearchRooms(ctx context.Context, query string, limit int, cursor string) ([]Room, string, error) {
	return s.repo.SearchRooms(ctx, strings.TrimSpace(query), effectiveLimit(limit), cursor)
}

// JoinRoom はメンバー追加。冪等に設計している:
// 既にメンバーなら ErrAlreadyMember を捕捉して成功扱い (「参加」ボタンの二度押しでエラーにしない)。
func (s *Service) JoinRoom(ctx context.Context, roomID string) error {
	userID, ok := auth.RequesterID(ctx)
	if !ok {
		return fmt.Errorf("%w: missing requester", ErrInvalidArgument)
	}
	if _, err := s.repo.GetRoom(ctx, roomID); err != nil {
		return err
	}
	err := s.repo.AddMember(ctx, roomID, userID)
	if err == ErrAlreadyMember {
		return nil
	}
	return err
}

// LeaveRoom は自分だけをメンバーから外す (他人を追放する API はない)。
// 対象が存在しなければ ErrNotMember を返すので、二重退出は FailedPrecondition になる。
func (s *Service) LeaveRoom(ctx context.Context, roomID string) error {
	userID, ok := auth.RequesterID(ctx)
	if !ok {
		return fmt.Errorf("%w: missing requester", ErrInvalidArgument)
	}
	return s.repo.RemoveMember(ctx, roomID, userID)
}

// ============================================================
// Internal API: 同一プロセス内の他ドメインから呼ばれる
// ============================================================
//
// Phase 2 の message.Service が認可判定に使う想定。gRPC として露出していない
// (chat-service プロセス内の Go 関数呼び出し)。

// EnsureMember は Phase 2 以降のメッセージ系 RPC が
// ルーム単位の操作を認可するために使う。
func (s *Service) EnsureMember(ctx context.Context, roomID, userID string) error {
	ok, err := s.repo.IsMember(ctx, roomID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotMember
	}
	return nil
}

// effectiveLimit はクライアント指定の limit をサーバー側で正規化する。
// 0 以下 → デフォルト、200 超 → 上限 200 に clamp。DoS 的な巨大 limit を拒否する狙い。
func effectiveLimit(limit int) int {
	if limit <= 0 {
		return defaultListLimit
	}
	if limit > 200 {
		return 200
	}
	return limit
}
