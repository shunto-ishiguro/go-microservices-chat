package room

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository は rooms / room_members のストレージを抽象化する。本ファイルに
// PostgreSQL 実装、テスト用の InMem 実装は repository_inmem.go にある。
//
// メンバー系の責務分担:
//
//	AddMember / RemoveMember : メンバー集合の変更 (Join / Leave)
//	ListMembers              : 一覧 (画面 #9 の enrich 前データ)
//	IsMember                 : boolean チェック (Phase 2 の EnsureMember 認可)
//	CountMembers             : 件数だけ (画面 #6 のヘッダ用、全件取得よりも軽い)
type Repository interface {
	CreateRoom(ctx context.Context, r *Room) error
	GetRoom(ctx context.Context, id string) (*Room, error)
	ListRoomsByMember(ctx context.Context, userID string, limit int, cursor string) ([]Room, string, error)
	SearchRooms(ctx context.Context, query string, limit int, cursor string) ([]Room, string, error)

	AddMember(ctx context.Context, roomID, userID string) error
	RemoveMember(ctx context.Context, roomID, userID string) error
	ListMembers(ctx context.Context, roomID string) ([]Member, error)
	IsMember(ctx context.Context, roomID, userID string) (bool, error)
	CountMembers(ctx context.Context, roomID string) (int, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// uniqueViolation は PostgreSQL SQLSTATE (unique_violation)。
// `room_members (room_id, user_id)` の UNIQUE 制約に引っかかった時の識別に使い、
// ErrAlreadyMember に変換する。
const uniqueViolation = "23505"

func (r *PostgresRepository) CreateRoom(ctx context.Context, room *Room) error {
	const q = `INSERT INTO rooms (id, name, created_by, created_at) VALUES ($1, $2, $3, $4)`
	_, err := r.pool.Exec(ctx, q, room.ID, room.Name, room.CreatedBy, room.CreatedAt)
	return err
}

func (r *PostgresRepository) GetRoom(ctx context.Context, id string) (*Room, error) {
	const q = `SELECT id, name, created_by, created_at FROM rooms WHERE id = $1`
	var out Room
	if err := r.pool.QueryRow(ctx, q, id).Scan(&out.ID, &out.Name, &out.CreatedBy, &out.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &out, nil
}

// ListRoomsByMember は指定ユーザーが参加している全ルームを新しい順で返す。
// room_members と rooms を JOIN するので userID ごとに 1 クエリで済む。
// Phase 1 はページネーション未実装 (cursor は無視)。Phase 2 で created_at < cursor で対応予定。
func (r *PostgresRepository) ListRoomsByMember(ctx context.Context, userID string, limit int, _ string) ([]Room, string, error) {
	const q = `
		SELECT r.id, r.name, r.created_by, r.created_at
		FROM rooms r
		JOIN room_members m ON m.room_id = r.id
		WHERE m.user_id = $1
		ORDER BY r.created_at DESC
		LIMIT $2
	`
	rows, err := r.pool.Query(ctx, q, userID, limit)
	if err != nil {
		return nil, "", err
	}
	return scanRooms(rows)
}

// SearchRooms は部分一致検索。ILIKE は大文字小文字を無視するマッチ (PostgreSQL 独自)。
// 学習用途なので全件スキャン相当で割り切り。実運用では pg_trgm や全文検索に切り替える。
func (r *PostgresRepository) SearchRooms(ctx context.Context, query string, limit int, _ string) ([]Room, string, error) {
	const q = `
		SELECT id, name, created_by, created_at
		FROM rooms
		WHERE name ILIKE '%' || $1 || '%'
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := r.pool.Query(ctx, q, query, limit)
	if err != nil {
		return nil, "", err
	}
	return scanRooms(rows)
}

func scanRooms(rows pgx.Rows) ([]Room, string, error) {
	defer rows.Close()
	var out []Room
	for rows.Next() {
		var r Room
		if err := rows.Scan(&r.ID, &r.Name, &r.CreatedBy, &r.CreatedAt); err != nil {
			return nil, "", err
		}
		out = append(out, r)
	}
	return out, "", rows.Err()
}

// AddMember は UNIQUE(room_id, user_id) 制約で「同じ人が 2 回 INSERT」を DB レベルで拒否する。
// アプリ側で事前 SELECT すると競合時に race が残るが、制約違反をキャッチする方が確実。
func (r *PostgresRepository) AddMember(ctx context.Context, roomID, userID string) error {
	const q = `INSERT INTO room_members (room_id, user_id) VALUES ($1, $2)`
	_, err := r.pool.Exec(ctx, q, roomID, userID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return ErrAlreadyMember
	}
	return err
}

func (r *PostgresRepository) RemoveMember(ctx context.Context, roomID, userID string) error {
	const q = `DELETE FROM room_members WHERE room_id = $1 AND user_id = $2`
	tag, err := r.pool.Exec(ctx, q, roomID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotMember
	}
	return nil
}

func (r *PostgresRepository) ListMembers(ctx context.Context, roomID string) ([]Member, error) {
	const q = `SELECT user_id, joined_at FROM room_members WHERE room_id = $1 ORDER BY joined_at ASC`
	rows, err := r.pool.Query(ctx, q, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) IsMember(ctx context.Context, roomID, userID string) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM room_members WHERE room_id = $1 AND user_id = $2)`
	var exists bool
	if err := r.pool.QueryRow(ctx, q, roomID, userID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// CountMembers は件数のみ返す (画面 #6 のヘッダで「メンバー数」を表示するためのもの)。
// ListMembers の結果を len するよりも、DB 側で COUNT した方が転送量が小さく
// (行を全部返さずに集約結果だけ返す)、大規模ルームで差が出る。
func (r *PostgresRepository) CountMembers(ctx context.Context, roomID string) (int, error) {
	const q = `SELECT COUNT(*) FROM room_members WHERE room_id = $1`
	var n int
	if err := r.pool.QueryRow(ctx, q, roomID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
