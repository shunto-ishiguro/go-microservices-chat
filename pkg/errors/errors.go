package errors

import (
	"errors"
	"fmt"
)

// よく使うエラーの定数定義
var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
	ErrValidation = errors.New("validation error")
	ErrInternal   = errors.New("internal error")
)

// AppError はエラーコードとフィールド詳細を持つアプリケーションエラー
type AppError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details []FieldError   `json:"details,omitempty"`
	Err     error          `json:"-"`
}

// FieldError は特定フィールドのバリデーションエラー
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap は内部エラーを返す（errors.Is/As で使われる）
func (e *AppError) Unwrap() error {
	return e.Err
}

// NewNotFound はリソースが見つからないエラーを生成する
func NewNotFound(resource, id string) *AppError {
	return &AppError{
		Code:    "NOT_FOUND",
		Message: fmt.Sprintf("%s with id '%s' not found", resource, id),
		Err:     ErrNotFound,
	}
}

// NewConflict は重複エラーを生成する（例: メールアドレスが既に存在）
func NewConflict(field, value string) *AppError {
	return &AppError{
		Code:    "CONFLICT",
		Message: fmt.Sprintf("%s '%s' already exists", field, value),
		Err:     ErrConflict,
	}
}

// NewValidation はバリデーションエラーを生成する
func NewValidation(details []FieldError) *AppError {
	return &AppError{
		Code:    "VALIDATION_ERROR",
		Message: "invalid input",
		Details: details,
		Err:     ErrValidation,
	}
}

// NewInternal は内部サーバーエラーを生成する（元のエラーをラップ）
func NewInternal(err error) *AppError {
	return &AppError{
		Code:    "INTERNAL_ERROR",
		Message: "an internal error occurred",
		Err:     fmt.Errorf("%w: %v", ErrInternal, err),
	}
}

// Is は errors.Is のラッパー
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// As は errors.As のラッパー
func As(err error, target any) bool {
	return errors.As(err, target)
}
