package room

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatv1 "go-microservices-chat/gen/go/chat/v1"
	"go-microservices-chat/services/chat-service/internal/userclient"
)

// GRPCAdapter は Room 関連 RPC を ChatServiceServer に適合させる。
// Phase 2 で Message 関連 RPC が加わった段階で internal/grpc/ に合流層を置く予定だが、
// Phase 1 では Room だけなので本アダプタ単体で ChatServiceServer を満たす。
//
// UnimplementedChatServiceServer の埋め込みは forward-compat のために必須。
// Message 系 RPC (SendMessage 等) は Phase 2 まで Unimplemented のデフォルト応答になる。
type GRPCAdapter struct {
	chatv1.UnimplementedChatServiceServer
	svc        *Service
	userClient userclient.Client
}

func NewGRPCAdapter(svc *Service, userClient userclient.Client) *GRPCAdapter {
	return &GRPCAdapter{svc: svc, userClient: userClient}
}

func (a *GRPCAdapter) CreateRoom(ctx context.Context, req *chatv1.CreateRoomRequest) (*chatv1.CreateRoomResponse, error) {
	r, err := a.svc.CreateRoom(ctx, req.GetName())
	if err != nil {
		return nil, mapError(err)
	}
	return &chatv1.CreateRoomResponse{Room: roomToProto(r, 1)}, nil
}

// GetRoom はルームの軽量情報のみ返す (画面 #6 のヘッダ用)。
// メンバー配列は返さない — チャット画面で本当に欲しいのは「ルーム名 / 作成者 / メンバー数」だけで、
// メンバー全員の display_name は必要ない。メンバー一覧が必要な画面 #9 では ListRoomMembers を
// 叩く設計にして、GetRoom 時の N+1 呼び出しを避けている。
func (a *GRPCAdapter) GetRoom(ctx context.Context, req *chatv1.GetRoomRequest) (*chatv1.GetRoomResponse, error) {
	r, count, err := a.svc.GetRoom(ctx, req.GetRoomId())
	if err != nil {
		return nil, mapError(err)
	}
	return &chatv1.GetRoomResponse{Room: roomToProto(r, count)}, nil
}

func (a *GRPCAdapter) ListRooms(ctx context.Context, req *chatv1.ListRoomsRequest) (*chatv1.ListRoomsResponse, error) {
	rooms, next, err := a.svc.ListMyRooms(ctx, int(req.GetLimit()), req.GetCursor())
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]*chatv1.Room, len(rooms))
	for i := range rooms {
		out[i] = roomToProto(&rooms[i], 0)
	}
	return &chatv1.ListRoomsResponse{Rooms: out, NextCursor: next}, nil
}

func (a *GRPCAdapter) SearchRooms(ctx context.Context, req *chatv1.SearchRoomsRequest) (*chatv1.SearchRoomsResponse, error) {
	rooms, next, err := a.svc.SearchRooms(ctx, req.GetQuery(), int(req.GetLimit()), req.GetCursor())
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]*chatv1.Room, len(rooms))
	for i := range rooms {
		out[i] = roomToProto(&rooms[i], 0)
	}
	return &chatv1.SearchRoomsResponse{Rooms: out, NextCursor: next}, nil
}

func (a *GRPCAdapter) JoinRoom(ctx context.Context, req *chatv1.JoinRoomRequest) (*chatv1.JoinRoomResponse, error) {
	if err := a.svc.JoinRoom(ctx, req.GetRoomId()); err != nil {
		return nil, mapError(err)
	}
	return &chatv1.JoinRoomResponse{}, nil
}

func (a *GRPCAdapter) LeaveRoom(ctx context.Context, req *chatv1.LeaveRoomRequest) (*chatv1.LeaveRoomResponse, error) {
	if err := a.svc.LeaveRoom(ctx, req.GetRoomId()); err != nil {
		return nil, mapError(err)
	}
	return &chatv1.LeaveRoomResponse{}, nil
}

// ListRoomMembers は画面 #9 用のメンバー一覧を返す。
// 手順: ①メンバー行を chatdb から取得 → ② enrichMembers で user-service を **1 回** 叩く →
// ③ 2 つを merge して proto に詰める。
// ② を個別 GetUser でループすると N+1 問題になるが、BatchGetUsers で 1 回に束ねて回避している。
func (a *GRPCAdapter) ListRoomMembers(ctx context.Context, req *chatv1.ListRoomMembersRequest) (*chatv1.ListRoomMembersResponse, error) {
	members, err := a.svc.ListRoomMembers(ctx, req.GetRoomId())
	if err != nil {
		return nil, mapError(err)
	}
	profiles, err := a.enrichMembers(ctx, members)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	out := make([]*chatv1.RoomMember, len(members))
	for i, m := range members {
		out[i] = memberToProto(m, profiles)
	}
	// Phase 1 はページネーション未実装。Phase 2 で cursor 対応予定。
	return &chatv1.ListRoomMembersResponse{Members: out, NextCursor: ""}, nil
}

// enrichMembers はメンバー全員の user_id を集めて BatchGetUsers を 1 回だけ呼ぶ。
// 素朴ループ (メンバーごとに GetUser) にすると N+1 になるのでここで束ねる。
func (a *GRPCAdapter) enrichMembers(ctx context.Context, members []Member) (map[string]userclient.Profile, error) {
	if len(members) == 0 {
		return nil, nil
	}
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = m.UserID
	}
	profiles, err := a.userClient.BatchGetUsers(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]userclient.Profile, len(profiles))
	for _, p := range profiles {
		byID[p.ID] = p
	}
	return byID, nil
}

func roomToProto(r *Room, count int) *chatv1.Room {
	return &chatv1.Room{
		Id:          r.ID,
		Name:        r.Name,
		CreatedBy:   r.CreatedBy,
		MemberCount: int32(count),
		CreatedAt:   timestamppb.New(r.CreatedAt),
	}
}

func memberToProto(m Member, profiles map[string]userclient.Profile) *chatv1.RoomMember {
	pm := &chatv1.RoomMember{
		UserId:   m.UserID,
		JoinedAt: timestamppb.New(m.JoinedAt),
	}
	if p, ok := profiles[m.UserID]; ok {
		pm.DisplayName = p.DisplayName
		pm.AvatarUrl = p.AvatarURL
	}
	return pm
}

func mapError(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, ErrAlreadyMember):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, ErrNotMember):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
