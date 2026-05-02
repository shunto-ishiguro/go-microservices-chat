// Package grpc は ChatServiceServer の合流層。
//
// Phase 2 から Room 系 / Message 系の RPC が並走するので、ドメイン別に Adapter を分け、
// このパッケージで両方を 1 つの Server に束ねて gRPC ランタイムに登録する。
//
// 実装上の注意: Phase 2 の docs に挙がっている「両 Adapter を struct embedding する」案は
// 両方の simple type name が `GRPCAdapter` で衝突するため Go では成立しない。
// 名前付きフィールドで保持し、各 RPC を明示的に forward する。
//
// UnimplementedChatServiceServer の埋め込みは forward-compat のために 1 箇所だけここで行う:
// 将来 RPC を proto に追加した瞬間に「対応する Adapter がまだ無い」状態でも
// chat-service プロセスがビルド可能なまま (= Unimplemented デフォルト応答) になる。
package grpc

import (
	"context"

	chatv1 "go-microservices-chat/gen/go/chat/v1"
	"go-microservices-chat/services/chat-service/internal/message"
	"go-microservices-chat/services/chat-service/internal/room"
)

// Server は ChatServiceServer 実装の合流点。
type Server struct {
	chatv1.UnimplementedChatServiceServer
	rooms    *room.GRPCAdapter
	messages *message.GRPCAdapter
}

func NewServer(rooms *room.GRPCAdapter, messages *message.GRPCAdapter) *Server {
	return &Server{rooms: rooms, messages: messages}
}

// --- Room 系 RPC: room.GRPCAdapter にそのまま委譲 ---

func (s *Server) CreateRoom(ctx context.Context, req *chatv1.CreateRoomRequest) (*chatv1.CreateRoomResponse, error) {
	return s.rooms.CreateRoom(ctx, req)
}

func (s *Server) GetRoom(ctx context.Context, req *chatv1.GetRoomRequest) (*chatv1.GetRoomResponse, error) {
	return s.rooms.GetRoom(ctx, req)
}

func (s *Server) ListRooms(ctx context.Context, req *chatv1.ListRoomsRequest) (*chatv1.ListRoomsResponse, error) {
	return s.rooms.ListRooms(ctx, req)
}

func (s *Server) SearchRooms(ctx context.Context, req *chatv1.SearchRoomsRequest) (*chatv1.SearchRoomsResponse, error) {
	return s.rooms.SearchRooms(ctx, req)
}

func (s *Server) JoinRoom(ctx context.Context, req *chatv1.JoinRoomRequest) (*chatv1.JoinRoomResponse, error) {
	return s.rooms.JoinRoom(ctx, req)
}

func (s *Server) LeaveRoom(ctx context.Context, req *chatv1.LeaveRoomRequest) (*chatv1.LeaveRoomResponse, error) {
	return s.rooms.LeaveRoom(ctx, req)
}

func (s *Server) ListRoomMembers(ctx context.Context, req *chatv1.ListRoomMembersRequest) (*chatv1.ListRoomMembersResponse, error) {
	return s.rooms.ListRoomMembers(ctx, req)
}

// --- Message 系 RPC: message.GRPCAdapter にそのまま委譲 ---

func (s *Server) SendMessage(ctx context.Context, req *chatv1.SendMessageRequest) (*chatv1.SendMessageResponse, error) {
	return s.messages.SendMessage(ctx, req)
}

func (s *Server) GetMessages(ctx context.Context, req *chatv1.GetMessagesRequest) (*chatv1.GetMessagesResponse, error) {
	return s.messages.GetMessages(ctx, req)
}
