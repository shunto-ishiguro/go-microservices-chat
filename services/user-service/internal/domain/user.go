package domain

import "time"

// User はユーザーエンティティ
type User struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	AvatarURL   string     `json:"avatar_url,omitempty"`
	StatusText  string     `json:"status_text,omitempty"`
	IsOnline    bool       `json:"is_online"`
	LastSeenAt  *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// CreateUserInput はユーザー作成時の入力データ
type CreateUserInput struct {
	Email       string `json:"email"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

// UpdateUserInput はユーザー更新時の入力データ（nilのフィールドは更新しない）
type UpdateUserInput struct {
	DisplayName *string `json:"display_name,omitempty"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
	StatusText  *string `json:"status_text,omitempty"`
}

// ListUsersParams はページネーションのパラメータ
type ListUsersParams struct {
	Limit  int
	Offset int
}
