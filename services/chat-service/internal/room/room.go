package room

import (
	"errors"
	"time"
)

// Room は公開チャットルーム。本プロジェクトではプライベートルームは扱わないので
// 可視性フラグは持たない。CreatedBy は user-service 所有のユーザー ID を参照するが、
// サービス境界を跨ぐので外部キー制約は張らない。
type Room struct {
	ID        string
	Name      string
	CreatedBy string
	CreatedAt time.Time
}

// Member はユーザーがルームに参加した記録。ロールは持たずフラット。
type Member struct {
	UserID   string
	JoinedAt time.Time
}

// ドメインエラー。grpc.go の mapError が gRPC status code に変換する:
//
//	ErrNotFound        → codes.NotFound          (ルーム or メンバー未存在)
//	ErrAlreadyMember   → codes.AlreadyExists     (JoinRoom では冪等扱いにして無視する)
//	ErrNotMember       → codes.FailedPrecondition (メンバーでないルームから退出等)
//	ErrInvalidArgument → codes.InvalidArgument   (空名の CreateRoom、未認証など)
var (
	ErrNotFound        = errors.New("room: not found")
	ErrAlreadyMember   = errors.New("room: already a member")
	ErrNotMember       = errors.New("room: not a member")
	ErrInvalidArgument = errors.New("room: invalid argument")
)
