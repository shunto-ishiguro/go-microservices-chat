package user

import (
	"errors"
	"time"
)

// User は user-service のドメインエンティティ。DB カラムとの対応は repository 層で処理する。
//
// PasswordHash は bcrypt ハッシュ済みの値。proto (gen/go/user/v1.User) には含めないので、
// クライアントには絶対に返さない (grpc.go の toProto で除外されている)。
type User struct {
	ID           string
	Email        string
	Username     string
	PasswordHash string
	DisplayName  string
	AvatarURL    string
	StatusText   string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// RefreshToken は発行済みリフレッシュトークンの失効・ローテーション用レコード。
// DB には sha256 ハッシュのみ保存し、生の値は発行時にクライアントへ 1 度だけ返す。
type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

// ドメインエラー。grpc.go の mapError が gRPC status code に変換する:
//
//	ErrNotFound      → codes.NotFound
//	ErrAlreadyExists → codes.AlreadyExists (email / username 重複)
//	ErrInvalidCreds  → codes.Unauthenticated (未認証 or パスワード不一致)
//	ErrTokenInvalid  → codes.Unauthenticated (refresh token が失効 or 期限切れ)
var (
	ErrNotFound      = errors.New("user: not found")
	ErrAlreadyExists = errors.New("user: already exists")
	ErrInvalidCreds  = errors.New("user: invalid credentials")
	ErrTokenInvalid  = errors.New("user: refresh token invalid or expired")
)
