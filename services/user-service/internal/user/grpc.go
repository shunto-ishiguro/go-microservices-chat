package user

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	userv1 "go-microservices-chat/gen/go/user/v1"
)

// GRPCAdapter は *Service を生成された UserServiceServer に適合させる。
// proto ↔ ドメイン型の変換と、ドメインエラー → gRPC status コードのマッピングのみを担う。
//
// UnimplementedUserServiceServer の埋め込みは forward-compat のために必須
// (proto に新 RPC が追加されてもコンパイルが通る)。
type GRPCAdapter struct {
	userv1.UnimplementedUserServiceServer
	svc *Service
}

func NewGRPCAdapter(svc *Service) *GRPCAdapter {
	return &GRPCAdapter{svc: svc}
}

// ============================================================
// External API: Web クライアントから呼ばれる (Envoy 経由)
// ============================================================

func (a *GRPCAdapter) Register(ctx context.Context, req *userv1.RegisterRequest) (*userv1.RegisterResponse, error) {
	u, err := a.svc.Register(ctx, req.GetEmail(), req.GetUsername(), req.GetDisplayName(), req.GetPassword())
	if err != nil {
		return nil, mapError(err)
	}
	return &userv1.RegisterResponse{User: toProto(u)}, nil
}

func (a *GRPCAdapter) Login(ctx context.Context, req *userv1.LoginRequest) (*userv1.LoginResponse, error) {
	access, refresh, err := a.svc.Login(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, mapError(err)
	}
	return &userv1.LoginResponse{AccessToken: access, RefreshToken: refresh}, nil
}

func (a *GRPCAdapter) Refresh(ctx context.Context, req *userv1.RefreshRequest) (*userv1.RefreshResponse, error) {
	access, refresh, err := a.svc.Refresh(ctx, req.GetRefreshToken())
	if err != nil {
		return nil, mapError(err)
	}
	return &userv1.RefreshResponse{AccessToken: access, RefreshToken: refresh}, nil
}

func (a *GRPCAdapter) GetMe(ctx context.Context, _ *userv1.GetMeRequest) (*userv1.GetMeResponse, error) {
	u, err := a.svc.GetMe(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	return &userv1.GetMeResponse{User: toProto(u)}, nil
}

func (a *GRPCAdapter) UpdateMe(ctx context.Context, req *userv1.UpdateMeRequest) (*userv1.UpdateMeResponse, error) {
	u, err := a.svc.UpdateMe(ctx, req.DisplayName, req.AvatarUrl, req.StatusText)
	if err != nil {
		return nil, mapError(err)
	}
	return &userv1.UpdateMeResponse{User: toProto(u)}, nil
}

// ============================================================
// Internal API: 他サービス (chat-service 等) から呼ばれる
// ============================================================

func (a *GRPCAdapter) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	u, err := a.svc.GetUser(ctx, req.GetUserId())
	if err != nil {
		return nil, mapError(err)
	}
	return &userv1.GetUserResponse{User: toProto(u)}, nil
}

func (a *GRPCAdapter) BatchGetUsers(ctx context.Context, req *userv1.BatchGetUsersRequest) (*userv1.BatchGetUsersResponse, error) {
	users, err := a.svc.BatchGetUsers(ctx, req.GetUserIds())
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]*userv1.User, len(users))
	for i := range users {
		out[i] = toProto(&users[i])
	}
	return &userv1.BatchGetUsersResponse{Users: out}, nil
}

// toProto はドメインの User を proto メッセージに変換する。
// PasswordHash は意図的に含めない (絶対にクライアントに漏らさない)。
func toProto(u *User) *userv1.User {
	return &userv1.User{
		Id:          u.ID,
		Email:       u.Email,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		AvatarUrl:   u.AvatarURL,
		StatusText:  u.StatusText,
		CreatedAt:   timestamppb.New(u.CreatedAt),
		UpdatedAt:   timestamppb.New(u.UpdatedAt),
	}
}

// mapError はドメインエラーを gRPC status code に変換する。
// 未分類のエラーは Internal に倒す (= クライアントには詳細を露出しない)。
func mapError(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, ErrInvalidCreds):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, ErrTokenInvalid):
		return status.Error(codes.Unauthenticated, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
