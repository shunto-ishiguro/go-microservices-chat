package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"go-microservices-chat/services/user-service/internal/domain"
)

// PostgresUserRepository は UserRepository の PostgreSQL 実装
type PostgresUserRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresUserRepository は PostgresUserRepository を生成する
func NewPostgresUserRepository(pool *pgxpool.Pool) *PostgresUserRepository {
	return &PostgresUserRepository{pool: pool}
}

// Create はユーザーをDBに保存する
func (r *PostgresUserRepository) Create(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO users (id, email, username, display_name, avatar_url, status_text, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := r.pool.Exec(ctx, query,
		user.ID, user.Email, user.Username, user.DisplayName,
		user.AvatarURL, user.StatusText, user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		// PostgreSQLのユニーク制約違反（コード23505）をチェック
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("unique constraint violation: %s: %w", pgErr.ConstraintName, err)
		}
		return err
	}
	return nil
}

// GetByID はIDでユーザーを取得する
func (r *PostgresUserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	return r.getByField(ctx, "id", id)
}

// GetByEmail はメールアドレスでユーザーを取得する
func (r *PostgresUserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.getByField(ctx, "email", email)
}

// GetByUsername はユーザー名でユーザーを取得する
func (r *PostgresUserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	return r.getByField(ctx, "username", username)
}

// getByField は指定カラムの値でユーザーを1件取得する共通メソッド
func (r *PostgresUserRepository) getByField(ctx context.Context, field, value string) (*domain.User, error) {
	query := fmt.Sprintf(`
		SELECT id, email, username, display_name, avatar_url, status_text,
		       is_online, last_seen_at, created_at, updated_at
		FROM users WHERE %s = $1`, field)

	var u domain.User
	err := r.pool.QueryRow(ctx, query, value).Scan(
		&u.ID, &u.Email, &u.Username, &u.DisplayName,
		&u.AvatarURL, &u.StatusText, &u.IsOnline, &u.LastSeenAt,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		// 該当行なしの場合は nil を返す（エラーではなく「見つからなかった」）
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// List はユーザー一覧をページネーション付きで取得する
func (r *PostgresUserRepository) List(ctx context.Context, limit, offset int) ([]*domain.User, int, error) {
	// まず総件数を取得
	var total int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// ユーザー一覧を作成日の降順で取得
	query := `
		SELECT id, email, username, display_name, avatar_url, status_text,
		       is_online, last_seen_at, created_at, updated_at
		FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2`

	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(
			&u.ID, &u.Email, &u.Username, &u.DisplayName,
			&u.AvatarURL, &u.StatusText, &u.IsOnline, &u.LastSeenAt,
			&u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		users = append(users, &u)
	}
	return users, total, rows.Err()
}

// Update はユーザー情報を更新する
func (r *PostgresUserRepository) Update(ctx context.Context, user *domain.User) error {
	query := `
		UPDATE users
		SET display_name = $2, avatar_url = $3, status_text = $4, updated_at = $5
		WHERE id = $1`

	tag, err := r.pool.Exec(ctx, query,
		user.ID, user.DisplayName, user.AvatarURL, user.StatusText, user.UpdatedAt,
	)
	if err != nil {
		return err
	}
	// 更新対象が0件 = 該当ユーザーが存在しない
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// Delete はユーザーを削除する
func (r *PostgresUserRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM users WHERE id = $1", id)
	if err != nil {
		return err
	}
	// 削除対象が0件 = 該当ユーザーが存在しない
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
