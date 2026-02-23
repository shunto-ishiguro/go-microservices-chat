package repository

import (
	"context"

	"go-microservices-chat/services/user-service/internal/domain"
)

// UserRepository はユーザーデータアクセスのインターフェース
// PostgreSQL実装やインメモリ実装など、具体的な実装を差し替え可能にする
type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	List(ctx context.Context, limit, offset int) ([]*domain.User, int, error)
	Update(ctx context.Context, user *domain.User) error
	Delete(ctx context.Context, id string) error
}
