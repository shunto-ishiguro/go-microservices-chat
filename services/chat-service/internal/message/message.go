package message

import (
	"errors"
	"time"
)

// Message はチャットメッセージ 1 件。SenderID はサービス境界を跨ぐ user-service 所有の
// ユーザー ID を指すので、Room と同様に外部キー制約は張らない (UUID 文字列で保持するだけ)。
type Message struct {
	ID        string
	RoomID    string
	SenderID  string
	Content   string
	CreatedAt time.Time
}

// ドメインエラー。grpc.go 側の mapError が gRPC status code に変換する:
//
//	ErrInvalidArgument → codes.InvalidArgument (空 content / 認証情報欠落 / sender 不一致)
//	ErrInvalidCursor   → codes.InvalidArgument (壊れた cursor 文字列)
var (
	ErrInvalidArgument = errors.New("message: invalid argument")
	ErrInvalidCursor   = errors.New("message: invalid cursor")
)
