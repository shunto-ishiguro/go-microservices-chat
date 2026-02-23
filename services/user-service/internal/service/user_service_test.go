package service

import (
	"context"
	"sync"
	"testing"

	apperrors "go-microservices-chat/pkg/errors"
	"go-microservices-chat/services/user-service/internal/domain"
)

// --- テスト用のインメモリリポジトリ ---
// PostgreSQLの代わりにmapでデータを保持する。テストでDB不要にするためのもの。

type memoryRepo struct {
	mu    sync.RWMutex
	users map[string]*domain.User
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{users: make(map[string]*domain.User)}
}

func (m *memoryRepo) Create(_ context.Context, user *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[user.ID] = user
	return nil
}

func (m *memoryRepo) GetByID(_ context.Context, id string) (*domain.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[id]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *memoryRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, nil
}

func (m *memoryRepo) GetByUsername(_ context.Context, username string) (*domain.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if u.Username == username {
			return u, nil
		}
	}
	return nil, nil
}

func (m *memoryRepo) List(_ context.Context, limit, offset int) ([]*domain.User, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	all := make([]*domain.User, 0, len(m.users))
	for _, u := range m.users {
		all = append(all, u)
	}

	total := len(all)
	if offset >= total {
		return []*domain.User{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (m *memoryRepo) Update(_ context.Context, user *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[user.ID]; !ok {
		return nil
	}
	m.users[user.ID] = user
	return nil
}

func (m *memoryRepo) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.users, id)
	return nil
}

// --- テスト ---

func TestCreateUser(t *testing.T) {
	tests := []struct {
		name    string
		input   domain.CreateUserInput
		wantErr bool
		errCode string
	}{
		{
			name: "正常系: 有効なユーザー",
			input: domain.CreateUserInput{
				Email:       "test@example.com",
				Username:    "testuser",
				DisplayName: "Test User",
			},
			wantErr: false,
		},
		{
			name: "異常系: メール空",
			input: domain.CreateUserInput{
				Email:       "",
				Username:    "testuser",
				DisplayName: "Test User",
			},
			wantErr: true,
			errCode: "VALIDATION_ERROR",
		},
		{
			name: "異常系: メール形式不正",
			input: domain.CreateUserInput{
				Email:       "not-an-email",
				Username:    "testuser",
				DisplayName: "Test User",
			},
			wantErr: true,
			errCode: "VALIDATION_ERROR",
		},
		{
			name: "異常系: ユーザー名空",
			input: domain.CreateUserInput{
				Email:       "test@example.com",
				Username:    "",
				DisplayName: "Test User",
			},
			wantErr: true,
			errCode: "VALIDATION_ERROR",
		},
		{
			name: "異常系: ユーザー名が短すぎる",
			input: domain.CreateUserInput{
				Email:       "test@example.com",
				Username:    "ab",
				DisplayName: "Test User",
			},
			wantErr: true,
			errCode: "VALIDATION_ERROR",
		},
		{
			name: "異常系: 表示名空",
			input: domain.CreateUserInput{
				Email:       "test@example.com",
				Username:    "testuser",
				DisplayName: "",
			},
			wantErr: true,
			errCode: "VALIDATION_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewUserService(newMemoryRepo())
			user, err := svc.Create(context.Background(), tt.input)

			if tt.wantErr {
				if err == nil {
					t.Fatal("エラーを期待したが nil だった")
				}
				var appErr *apperrors.AppError
				if apperrors.As(err, &appErr) {
					if appErr.Code != tt.errCode {
						t.Errorf("エラーコード: 期待=%q, 実際=%q", tt.errCode, appErr.Code)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("予期しないエラー: %v", err)
			}
			if user.ID == "" {
				t.Error("ユーザーIDが設定されていない")
			}
			if user.Email != tt.input.Email {
				t.Errorf("メール: 期待=%q, 実際=%q", tt.input.Email, user.Email)
			}
		})
	}
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	svc := NewUserService(newMemoryRepo())
	ctx := context.Background()

	input := domain.CreateUserInput{
		Email:       "dup@example.com",
		Username:    "user1",
		DisplayName: "User One",
	}
	_, err := svc.Create(ctx, input)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}

	// 同じメールアドレスで2回目の作成 → 重複エラーになるべき
	input.Username = "user2"
	_, err = svc.Create(ctx, input)
	if err == nil {
		t.Fatal("メール重複でエラーを期待したが nil だった")
	}
	if !apperrors.Is(err, apperrors.ErrConflict) {
		t.Errorf("ErrConflict を期待したが: %v", err)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	svc := NewUserService(newMemoryRepo())
	_, err := svc.GetByID(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("NotFoundエラーを期待したが nil だった")
	}
	if !apperrors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("ErrNotFound を期待したが: %v", err)
	}
}

func TestUpdateUser(t *testing.T) {
	svc := NewUserService(newMemoryRepo())
	ctx := context.Background()

	user, err := svc.Create(ctx, domain.CreateUserInput{
		Email:       "update@example.com",
		Username:    "updateuser",
		DisplayName: "Original",
	})
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}

	newName := "Updated Name"
	updated, err := svc.Update(ctx, user.ID, domain.UpdateUserInput{
		DisplayName: &newName,
	})
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if updated.DisplayName != newName {
		t.Errorf("表示名: 期待=%q, 実際=%q", newName, updated.DisplayName)
	}
}

func TestDeleteUser(t *testing.T) {
	svc := NewUserService(newMemoryRepo())
	ctx := context.Background()

	user, err := svc.Create(ctx, domain.CreateUserInput{
		Email:       "delete@example.com",
		Username:    "deleteuser",
		DisplayName: "Delete Me",
	})
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}

	err = svc.Delete(ctx, user.ID)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}

	// 削除後に取得 → NotFound になるべき
	_, err = svc.GetByID(ctx, user.ID)
	if !apperrors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("削除後に ErrNotFound を期待したが: %v", err)
	}
}

func TestListUsers(t *testing.T) {
	svc := NewUserService(newMemoryRepo())
	ctx := context.Background()

	// 5人のユーザーを作成
	for i := 0; i < 5; i++ {
		_, err := svc.Create(ctx, domain.CreateUserInput{
			Email:       "list" + string(rune('a'+i)) + "@example.com",
			Username:    "listuser" + string(rune('a'+i)),
			DisplayName: "List User",
		})
		if err != nil {
			t.Fatalf("ユーザー作成中の予期しないエラー（%d人目）: %v", i, err)
		}
	}

	// Limit=3 で取得 → 3人だけ返り、total は 5
	users, total, err := svc.List(ctx, domain.ListUsersParams{Limit: 3, Offset: 0})
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if total != 5 {
		t.Errorf("総数: 期待=5, 実際=%d", total)
	}
	if len(users) != 3 {
		t.Errorf("取得件数: 期待=3, 実際=%d", len(users))
	}
}
