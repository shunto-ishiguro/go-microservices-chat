package message

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository は messages テーブルへの読み書きを抽象化する。
// 本ファイルに PostgreSQL 実装、テスト用の InMem 実装は repository_inmem.go にある。
//
// Insert は INSERT、ListByRoom は cursor (created_at, id) より古いものを新しい順で N 件返す。
// 外部キー制約は持たない設計なので Repository 単独でメッセージの整合性を取る必要はない
// (発信元の認可・room 存在チェックは Service / GRPCAdapter 側で済ませる)。
type Repository interface {
	Insert(ctx context.Context, m *Message) error
	// ListByRoom は cursor より古いメッセージを created_at の降順で limit 件返す。
	// cursor が zero value なら「最新から」。
	ListByRoom(ctx context.Context, roomID string, limit int, cursor Cursor) ([]Message, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Insert(ctx context.Context, m *Message) error {
	const q = `INSERT INTO messages (id, room_id, sender_id, content, created_at) VALUES ($1, $2, $3, $4, $5)`
	_, err := r.pool.Exec(ctx, q, m.ID, m.RoomID, m.SenderID, m.Content, m.CreatedAt)
	return err
}

// ListByRoom は (created_at, id) のタプル比較でカーソルを実現する。
// 同一秒に複数行ある時に取りこぼし / 重複が起きないよう id まで含めて比較するのがポイント。
func (r *PostgresRepository) ListByRoom(ctx context.Context, roomID string, limit int, cursor Cursor) ([]Message, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if cursor.IsZero() {
		const q = `
			SELECT id, room_id, sender_id, content, created_at
			FROM messages
			WHERE room_id = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2
		`
		rows, err = r.pool.Query(ctx, q, roomID, limit)
	} else {
		const q = `
			SELECT id, room_id, sender_id, content, created_at
			FROM messages
			WHERE room_id = $1
			  AND (created_at, id) < ($2, $3)
			ORDER BY created_at DESC, id DESC
			LIMIT $4
		`
		rows, err = r.pool.Query(ctx, q, roomID, cursor.CreatedAt, cursor.ID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.RoomID, &m.SenderID, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
