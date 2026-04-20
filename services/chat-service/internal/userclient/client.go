package userclient

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	userv1 "go-microservices-chat/gen/go/user/v1"
	"go-microservices-chat/pkg/auth"
)

// Profile は chat-service がレスポンスを enrich する時に必要な
// ユーザー情報の部分集合 (例: GetRoom でメンバーの表示名を差し込む用途)。
type Profile struct {
	ID          string
	Username    string
	DisplayName string
	AvatarURL   string
}

// Client は user-service への gRPC クライアントを interface で隠蔽する。
// テストでは fake に差し替えて、実際の接続なしで検証できる。
type Client interface {
	GetUser(ctx context.Context, userID string) (*Profile, error)
	// BatchGetUsers は複数 ID を 1 回で取得する (N+1 回避)。
	// 存在しない ID は結果から欠落する。
	BatchGetUsers(ctx context.Context, userIDs []string) ([]Profile, error)
	Close() error
}

type grpcClient struct {
	conn *grpc.ClientConn
	svc  userv1.UserServiceClient
}

// Dial は user-service への長寿命 gRPC 接続を開く。呼び出し元は defer Close すること。
//
// gRPC の ClientConn は内部で HTTP/2 の多重化をするので、1 本の接続を全リクエストで
// 使い回すのが正しい使い方 (Dial のたびに TCP + TLS ハンドシェイクするのは高コスト)。
// 本サービスでは main で 1 回だけ Dial してプロセス寿命を通じて保持する。
func Dial(addr string) (Client, error) {
	if addr == "" {
		return nil, fmt.Errorf("userclient: addr is required")
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("userclient: dial: %w", err)
	}
	return &grpcClient{conn: conn, svc: userv1.NewUserServiceClient(conn)}, nil
}

func (c *grpcClient) GetUser(ctx context.Context, userID string) (*Profile, error) {
	ctx = auth.PropagateRequester(ctx)
	resp, err := c.svc.GetUser(ctx, &userv1.GetUserRequest{UserId: userID})
	if err != nil {
		return nil, err
	}
	return toProfile(resp.GetUser()), nil
}

func (c *grpcClient) BatchGetUsers(ctx context.Context, userIDs []string) ([]Profile, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	// 下流 (user-service) に x-user-id を伝搬しないと、あちら側で RequesterID(ctx) が
	// 引けない。incoming → outgoing の詰め替えは pkg/auth のヘルパーで行う。
	ctx = auth.PropagateRequester(ctx)
	resp, err := c.svc.BatchGetUsers(ctx, &userv1.BatchGetUsersRequest{UserIds: userIDs})
	if err != nil {
		return nil, err
	}
	out := make([]Profile, 0, len(resp.GetUsers()))
	for _, u := range resp.GetUsers() {
		out = append(out, *toProfile(u))
	}
	return out, nil
}

func (c *grpcClient) Close() error {
	return c.conn.Close()
}

func toProfile(u *userv1.User) *Profile {
	return &Profile{
		ID:          u.GetId(),
		Username:    u.GetUsername(),
		DisplayName: u.GetDisplayName(),
		AvatarURL:   u.GetAvatarUrl(),
	}
}
