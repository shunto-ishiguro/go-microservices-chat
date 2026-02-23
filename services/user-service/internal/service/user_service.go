package service

import (
	"context"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	apperrors "go-microservices-chat/pkg/errors"
	"go-microservices-chat/services/user-service/internal/domain"
	"go-microservices-chat/services/user-service/internal/repository"
)

// UserService はユーザーに関するビジネスロジックを担当する
type UserService struct {
	repo repository.UserRepository
}

// NewUserService は UserService を生成する
func NewUserService(repo repository.UserRepository) *UserService {
	return &UserService{repo: repo}
}

// Create はバリデーション後に新しいユーザーを作成する
func (s *UserService) Create(ctx context.Context, input domain.CreateUserInput) (*domain.User, error) {
	if errs := validateCreateInput(input); len(errs) > 0 {
		return nil, apperrors.NewValidation(errs)
	}

	// メールアドレスの重複チェック
	existing, err := s.repo.GetByEmail(ctx, input.Email)
	if err != nil {
		return nil, apperrors.NewInternal(err)
	}
	if existing != nil {
		return nil, apperrors.NewConflict("email", input.Email)
	}

	// ユーザー名の重複チェック
	existing, err = s.repo.GetByUsername(ctx, input.Username)
	if err != nil {
		return nil, apperrors.NewInternal(err)
	}
	if existing != nil {
		return nil, apperrors.NewConflict("username", input.Username)
	}

	now := time.Now().UTC()
	user := &domain.User{
		ID:          uuid.New().String(),
		Email:       input.Email,
		Username:    input.Username,
		DisplayName: input.DisplayName,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, apperrors.NewInternal(err)
	}

	return user, nil
}

// GetByID はIDでユーザーを取得する
func (s *UserService) GetByID(ctx context.Context, id string) (*domain.User, error) {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, apperrors.NewInternal(err)
	}
	if user == nil {
		return nil, apperrors.NewNotFound("user", id)
	}
	return user, nil
}

// List はページネーション付きでユーザー一覧を取得する
func (s *UserService) List(ctx context.Context, params domain.ListUsersParams) ([]*domain.User, int, error) {
	if params.Limit <= 0 {
		params.Limit = 20
	}
	if params.Limit > 100 {
		params.Limit = 100
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	users, total, err := s.repo.List(ctx, params.Limit, params.Offset)
	if err != nil {
		return nil, 0, apperrors.NewInternal(err)
	}
	return users, total, nil
}

// Update は既存ユーザーの情報を更新する
func (s *UserService) Update(ctx context.Context, id string, input domain.UpdateUserInput) (*domain.User, error) {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, apperrors.NewInternal(err)
	}
	if user == nil {
		return nil, apperrors.NewNotFound("user", id)
	}

	// 送られてきたフィールドだけ更新する
	if input.DisplayName != nil {
		if errs := validateDisplayName(*input.DisplayName); len(errs) > 0 {
			return nil, apperrors.NewValidation(errs)
		}
		user.DisplayName = *input.DisplayName
	}
	if input.AvatarURL != nil {
		user.AvatarURL = *input.AvatarURL
	}
	if input.StatusText != nil {
		user.StatusText = *input.StatusText
	}

	user.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, user); err != nil {
		return nil, apperrors.NewInternal(err)
	}

	return user, nil
}

// Delete はIDでユーザーを削除する
func (s *UserService) Delete(ctx context.Context, id string) error {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return apperrors.NewInternal(err)
	}
	if user == nil {
		return apperrors.NewNotFound("user", id)
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return apperrors.NewInternal(err)
	}
	return nil
}

// validateCreateInput はユーザー作成時の入力をバリデーションする
func validateCreateInput(input domain.CreateUserInput) []apperrors.FieldError {
	var errs []apperrors.FieldError

	input.Email = strings.TrimSpace(input.Email)
	if input.Email == "" {
		errs = append(errs, apperrors.FieldError{Field: "email", Message: "is required"})
	} else if _, err := mail.ParseAddress(input.Email); err != nil {
		errs = append(errs, apperrors.FieldError{Field: "email", Message: "must be a valid email address"})
	}

	input.Username = strings.TrimSpace(input.Username)
	if input.Username == "" {
		errs = append(errs, apperrors.FieldError{Field: "username", Message: "is required"})
	} else if utf8.RuneCountInString(input.Username) < 3 || utf8.RuneCountInString(input.Username) > 50 {
		errs = append(errs, apperrors.FieldError{Field: "username", Message: "must be between 3 and 50 characters"})
	}

	errs = append(errs, validateDisplayName(input.DisplayName)...)

	return errs
}

// validateDisplayName は表示名のバリデーション
func validateDisplayName(name string) []apperrors.FieldError {
	var errs []apperrors.FieldError
	name = strings.TrimSpace(name)
	if name == "" {
		errs = append(errs, apperrors.FieldError{Field: "display_name", Message: "is required"})
	} else if utf8.RuneCountInString(name) > 100 {
		errs = append(errs, apperrors.FieldError{Field: "display_name", Message: "must be at most 100 characters"})
	}
	return errs
}
