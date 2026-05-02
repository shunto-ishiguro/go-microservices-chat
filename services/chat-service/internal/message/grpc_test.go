package message_test

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	chatv1 "go-microservices-chat/gen/go/chat/v1"
	authpkg "go-microservices-chat/pkg/auth"
	chatgrpc "go-microservices-chat/services/chat-service/internal/grpc"
	"go-microservices-chat/services/chat-service/internal/message"
	"go-microservices-chat/services/chat-service/internal/room"
	"go-microservices-chat/services/chat-service/internal/userclient"
)

// startServer は Phase 2 の合流層 (chatgrpc.Server) を bufconn 上で起動する。
// 全 RPC を 1 つの ChatServiceClient で叩けるので、CreateRoom → JoinRoom → SendMessage の
// シナリオを RPC 経由で連結できる。
func startServer(t *testing.T) (chatv1.ChatServiceClient, func()) {
	t.Helper()
	rooms := room.NewService(room.NewInMemRepository())
	messages := message.NewService(message.NewInMemRepository())

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	chatv1.RegisterChatServiceServer(srv, chatgrpc.NewServer(
		room.NewGRPCAdapter(rooms, userclient.NewFake()),
		message.NewGRPCAdapter(messages, rooms),
	))
	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return chatv1.NewChatServiceClient(conn), func() {
		conn.Close()
		srv.Stop()
	}
}

func ctxAs(userID string) context.Context {
	return metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs(authpkg.MetadataKeyUserID, userID))
}

func TestGRPC_SendMessage_RequiresMembership(t *testing.T) {
	client, cleanup := startServer(t)
	defer cleanup()

	created, err := client.CreateRoom(ctxAs("alice"), &chatv1.CreateRoomRequest{Name: "general"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	roomID := created.GetRoom().GetId()

	// Bob はメンバーじゃないので PermissionDenied
	_, err = client.SendMessage(ctxAs("bob"), &chatv1.SendMessageRequest{RoomId: roomID, SenderId: "bob", Content: "hi"})
	if code := status.Code(err); code != codes.PermissionDenied {
		t.Errorf("non-member code = %s, want PermissionDenied", code)
	}

	// Alice は作成者なので即送信できる
	resp, err := client.SendMessage(ctxAs("alice"), &chatv1.SendMessageRequest{RoomId: roomID, SenderId: "alice", Content: "hi"})
	if err != nil {
		t.Fatalf("alice send: %v", err)
	}
	if resp.GetMessage().GetSenderId() != "alice" || resp.GetMessage().GetContent() != "hi" {
		t.Errorf("unexpected response: %+v", resp.GetMessage())
	}
}

func TestGRPC_SendMessage_RejectsImpersonation(t *testing.T) {
	// x-user-id (alice) と SendMessageRequest.SenderID (bob) が食い違う = なりすまし試行
	client, cleanup := startServer(t)
	defer cleanup()
	created, _ := client.CreateRoom(ctxAs("alice"), &chatv1.CreateRoomRequest{Name: "general"})
	_, err := client.SendMessage(ctxAs("alice"), &chatv1.SendMessageRequest{
		RoomId: created.GetRoom().GetId(), SenderId: "bob", Content: "spoofed",
	})
	if code := status.Code(err); code != codes.PermissionDenied {
		t.Errorf("code = %s, want PermissionDenied", code)
	}
}

func TestGRPC_SendMessage_Unauthenticated(t *testing.T) {
	client, cleanup := startServer(t)
	defer cleanup()
	created, _ := client.CreateRoom(ctxAs("alice"), &chatv1.CreateRoomRequest{Name: "general"})
	_, err := client.SendMessage(context.Background(), &chatv1.SendMessageRequest{
		RoomId: created.GetRoom().GetId(), Content: "hi",
	})
	if code := status.Code(err); code != codes.Unauthenticated {
		t.Errorf("code = %s, want Unauthenticated", code)
	}
}

func TestGRPC_GetMessages_ReturnsHistoryNewestFirst(t *testing.T) {
	client, cleanup := startServer(t)
	defer cleanup()

	created, _ := client.CreateRoom(ctxAs("alice"), &chatv1.CreateRoomRequest{Name: "general"})
	roomID := created.GetRoom().GetId()
	for _, c := range []string{"first", "second", "third"} {
		if _, err := client.SendMessage(ctxAs("alice"), &chatv1.SendMessageRequest{
			RoomId: roomID, SenderId: "alice", Content: c,
		}); err != nil {
			t.Fatalf("send %q: %v", c, err)
		}
	}

	resp, err := client.GetMessages(ctxAs("alice"), &chatv1.GetMessagesRequest{RoomId: roomID})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got := resp.GetMessages()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].GetContent() != "third" || got[2].GetContent() != "first" {
		t.Errorf("ordering broken: %+v", got)
	}
}

func TestGRPC_GetMessages_NonMemberDenied(t *testing.T) {
	client, cleanup := startServer(t)
	defer cleanup()
	created, _ := client.CreateRoom(ctxAs("alice"), &chatv1.CreateRoomRequest{Name: "general"})
	_, err := client.GetMessages(ctxAs("bob"), &chatv1.GetMessagesRequest{RoomId: created.GetRoom().GetId()})
	if code := status.Code(err); code != codes.PermissionDenied {
		t.Errorf("code = %s, want PermissionDenied", code)
	}
}
