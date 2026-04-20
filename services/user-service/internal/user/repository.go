package user

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository は Service が依存するストレージインターフェイス。このパッケージには
// 2 実装がある: 下の PostgreSQL 実装と、repository_inmem.go のインメモリ実装
// (DB なしで `go test ./...` を通すために使う)。
type Repository interface {
	CreateUser(ctx context.Context, u *User) error
	GetUserByID(ctx context.Context, id string) (*User, error)
	GetUsersByIDs(ctx context.Context, ids []string) ([]User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUser(ctx context.Context, u *User) error

	CreateRefreshToken(ctx context.Context, t *RefreshToken) error
	GetRefreshTokenByHash(ctx context.Context, hash string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id string, revokedAt time.Time) error
}

// PostgresRepository は pgx 経由でユーザーとリフレッシュトークンを PostgreSQL に永続化する。
type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// uniqueViolation は PostgreSQL の SQLSTATE (unique_violation)。
// 重複キーで INSERT した時にこのコードが返るので、ドメインエラーの ErrAlreadyExists に変換する。
// https://www.postgresql.org/docs/current/errcodes-appendix.html
const uniqueViolation = "23505"

func (r *PostgresRepository) CreateUser(ctx context.Context, u *User) error {
	const q = `
		INSERT INTO users (id, email, username, password_hash, display_name, avatar_url, status_text, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := r.pool.Exec(ctx, q,
		u.ID, u.Email, u.Username, u.PasswordHash, u.DisplayName,
		nullString(u.AvatarURL), nullString(u.StatusText),
		u.CreatedAt, u.UpdatedAt,
	)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return ErrAlreadyExists
	}
	return err
}

func (r *PostgresRepository) GetUserByID(ctx context.Context, id string) (*User, error) {
	return r.getUser(ctx, `WHERE id = $1`, id)
}

// GetUsersByIDs は `WHERE id = ANY($1)` で 1 クエリに束ねて N+1 を回避する。
// 存在しない ID は結果から欠落する (エラーにはしない)。
//
// pgx は []string を PostgreSQL の配列型にバインドしてくれるので、
// IN (?, ?, ?, ...) 相当の動的プレースホルダ生成は不要 (= SQL injection も気にしなくていい)。
func (r *PostgresRepository) GetUsersByIDs(ctx context.Context, ids []string) ([]User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	const q = `
		SELECT id, email, username, password_hash, display_name,
			COALESCE(avatar_url, ''), COALESCE(status_text, ''),
			created_at, updated_at
		FROM users
		WHERE id = ANY($1)
	`
	rows, err := r.pool.Query(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.DisplayName,
			&u.AvatarURL, &u.StatusText, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return r.getUser(ctx, `WHERE email = $1`, email)
}

// getUser は GetUserByID / GetUserByEmail の共通実装。WHERE 句だけ差し替える。
//
// COALESCE(avatar_url, '') は nullable 列を Go の string ゼロ値として扱うため。
// 生のポインタで受けると呼び出し側で nil チェックが必要になり取り回しが悪い。
func (r *PostgresRepository) getUser(ctx context.Context, where string, arg any) (*User, error) {
	q := `SELECT id, email, username, password_hash, display_name,
		COALESCE(avatar_url, ''), COALESCE(status_text, ''),
		created_at, updated_at FROM users ` + where
	row := r.pool.QueryRow(ctx, q, arg)
	var u User
	if err := row.Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.DisplayName,
		&u.AvatarURL, &u.StatusText, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *PostgresRepository) UpdateUser(ctx context.Context, u *User) error {
	const q = `
		UPDATE users
		SET display_name = $2, avatar_url = $3, status_text = $4, updated_at = $5
		WHERE id = $1
	`
	tag, err := r.pool.Exec(ctx, q,
		u.ID, u.DisplayName, nullString(u.AvatarURL), nullString(u.StatusText), u.UpdatedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) CreateRefreshToken(ctx context.Context, t *RefreshToken) error {
	const q = `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.pool.Exec(ctx, q, t.ID, t.UserID, t.TokenHash, t.ExpiresAt, t.CreatedAt)
	return err
}

func (r *PostgresRepository) GetRefreshTokenByHash(ctx context.Context, hash string) (*RefreshToken, error) {
	const q = `
		SELECT id, user_id, token_hash, expires_at, revoked_at, created_at
		FROM refresh_tokens WHERE token_hash = $1
	`
	row := r.pool.QueryRow(ctx, q, hash)
	var t RefreshToken
	if err := row.Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *PostgresRepository) RevokeRefreshToken(ctx context.Context, id string, revokedAt time.Time) error {
	const q = `UPDATE refresh_tokens SET revoked_at = $2 WHERE id = $1`
	tag, err := r.pool.Exec(ctx, q, id, revokedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// nullString は Go 側のゼロ値 "" を SQL の NULL にマップする。
// 空文字を明示的に保存するケースが無いので「未設定」= NULL で統一する。
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
