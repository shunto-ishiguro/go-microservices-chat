package message

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatv1 "go-microservices-chat/gen/go/chat/v1"
	"go-microservices-chat/pkg/auth"
	"go-microservices-chat/services/chat-service/internal/room"
)

// GRPCAdapter は Message 系 RPC を提供する。
//
// 認可の三段構え:
//  1. auth.RequesterID(ctx) で「呼び出し元ユーザー」を確定 (Envoy が x-user-id を注入済み)
//  2. SendMessageRequest.SenderID と requester の一致を強制 (なりすまし防止)
//  3. room.Service.EnsureMember でメンバーシップ確認 (Room ↔ Message の唯一の横断点)
type GRPCAdapter struct {
	svc   *Service
	rooms *room.Service
}

func NewGRPCAdapter(svc *Service, rooms *room.Service) *GRPCAdapter {
	return &GRPCAdapter{svc: svc, rooms: rooms}
}

func (a *GRPCAdapter) SendMessage(ctx context.Context, req *chatv1.SendMessageRequest) (*chatv1.SendMessageResponse, error) {
	requester, ok := auth.RequesterID(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing x-user-id")
	}
	// SenderID は realtime-service から渡される。Envoy 経由で確認済みの requester と一致しなければ拒否。
	// (なりすまし防止: A が x-user-id を Bob で来ながら sender_id=Charlie を送れない)
	if req.GetSenderId() != "" && req.GetSenderId() != requester {
		return nil, status.Error(codes.PermissionDenied, "sender_id does not match authenticated user")
	}
	if err := a.rooms.EnsureMember(ctx, req.GetRoomId(), requester); err != nil {
		return nil, mapError(err)
	}
	m, err := a.svc.Send(ctx, req.GetRoomId(), requester, req.GetContent())
	if err != nil {
		return nil, mapError(err)
	}
	return &chatv1.SendMessageResponse{Message: toProto(m)}, nil
}

func (a *GRPCAdapter) GetMessages(ctx context.Context, req *chatv1.GetMessagesRequest) (*chatv1.GetMessagesResponse, error) {
	requester, ok := auth.RequesterID(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing x-user-id")
	}
	if err := a.rooms.EnsureMember(ctx, req.GetRoomId(), requester); err != nil {
		return nil, mapError(err)
	}
	msgs, next, err := a.svc.GetMessages(ctx, req.GetRoomId(), int(req.GetLimit()), req.GetCursor())
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]*chatv1.Message, len(msgs))
	for i := range msgs {
		out[i] = toProto(&msgs[i])
	}
	return &chatv1.GetMessagesResponse{Messages: out, NextCursor: next}, nil
}

func toProto(m *Message) *chatv1.Message {
	return &chatv1.Message{
		Id:        m.ID,
		RoomId:    m.RoomID,
		SenderId:  m.SenderID,
		Content:   m.Content,
		CreatedAt: timestamppb.New(m.CreatedAt),
	}
}

// mapError は Message ドメイン + Room ドメイン (EnsureMember 経由) のエラーを gRPC code に変換する。
// Room 側のエラーも触るので room パッケージのドメインエラーをここで参照する。
func mapError(err error) error {
	switch {
	case errors.Is(err, ErrInvalidArgument), errors.Is(err, ErrInvalidCursor):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, room.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, room.ErrNotMember):
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
