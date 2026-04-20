package room_test

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
	"go-microservices-chat/services/chat-service/internal/room"
	"go-microservices-chat/services/chat-service/internal/userclient"
)

func startServer(t *testing.T) (chatv1.ChatServiceClient, *userclient.Fake, func()) {
	t.Helper()
	repo := room.NewInMemRepository()
	rooms := room.NewService(repo)
	fake := userclient.NewFake()

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	chatv1.RegisterChatServiceServer(srv, room.NewGRPCAdapter(rooms, fake))
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
	cleanup := func() {
		conn.Close()
		srv.Stop()
	}
	return chatv1.NewChatServiceClient(conn), fake, cleanup
}

// outgoingCtxAs は gRPC **クライアント** 側として x-user-id を載せた context を作る。
// server_test.go の ctxAs (NewIncomingContext) とは向きが違うので別関数にしている。
func outgoingCtxAs(userID string) context.Context {
	return metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs(authpkg.MetadataKeyUserID, userID))
}

func TestGRPCAdapter_CreateJoinGetRoom_HeaderOnly(t *testing.T) {
	// GetRoom は画面 #6 のヘッダ用なのでルーム情報 + member_count のみ返す。
	// メンバー配列は ListRoomMembers で取る。
	client, _, cleanup := startServer(t)
	defer cleanup()

	created, err := client.CreateRoom(outgoingCtxAs("alice-uuid"), &chatv1.CreateRoomRequest{Name: "general"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	roomID := created.GetRoom().GetId()
	if created.GetRoom().GetCreatedBy() != "alice-uuid" {
		t.Errorf("created_by = %q", created.GetRoom().GetCreatedBy())
	}
	if created.GetRoom().GetMemberCount() != 1 {
		t.Errorf("member_count = %d", created.GetRoom().GetMemberCount())
	}

	if _, err := client.JoinRoom(outgoingCtxAs("bob-uuid"), &chatv1.JoinRoomRequest{RoomId: roomID}); err != nil {
		t.Fatalf("join: %v", err)
	}

	got, err := client.GetRoom(outgoingCtxAs("bob-uuid"), &chatv1.GetRoomRequest{RoomId: roomID})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.GetRoom().GetMemberCount() != 2 {
		t.Errorf("member_count = %d, want 2", got.GetRoom().GetMemberCount())
	}
}

func TestGRPCAdapter_ListRoomMembers_Enriches(t *testing.T) {
	client, fake, cleanup := startServer(t)
	defer cleanup()

	fake.Set(&userclient.Profile{ID: "alice-uuid", Username: "alice", DisplayName: "Alice", AvatarURL: "https://cdn/alice.png"})
	fake.Set(&userclient.Profile{ID: "bob-uuid", Username: "bob", DisplayName: "Bob", AvatarURL: "https://cdn/bob.png"})

	created, _ := client.CreateRoom(outgoingCtxAs("alice-uuid"), &chatv1.CreateRoomRequest{Name: "general"})
	roomID := created.GetRoom().GetId()
	if _, err := client.JoinRoom(outgoingCtxAs("bob-uuid"), &chatv1.JoinRoomRequest{RoomId: roomID}); err != nil {
		t.Fatalf("join: %v", err)
	}

	resp, err := client.ListRoomMembers(outgoingCtxAs("alice-uuid"), &chatv1.ListRoomMembersRequest{RoomId: roomID})
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	members := resp.GetMembers()
	if len(members) != 2 {
		t.Fatalf("members len = %d", len(members))
	}
	byID := map[string]*chatv1.RoomMember{}
	for _, m := range members {
		byID[m.GetUserId()] = m
	}
	if got := byID["alice-uuid"]; got == nil || got.GetDisplayName() != "Alice" || got.GetAvatarUrl() != "https://cdn/alice.png" {
		t.Errorf("alice not enriched: %+v", got)
	}
	if got := byID["bob-uuid"]; got == nil || got.GetDisplayName() != "Bob" || got.GetAvatarUrl() != "https://cdn/bob.png" {
		t.Errorf("bob not enriched: %+v", got)
	}
}

func TestGRPCAdapter_ListRoomMembers_MissingProfileIsOK(t *testing.T) {
	// profile が user-service に無いメンバーがいても、user_id のみ返して続行する (部分成功)。
	client, _, cleanup := startServer(t)
	defer cleanup()

	created, _ := client.CreateRoom(outgoingCtxAs("alice-uuid"), &chatv1.CreateRoomRequest{Name: "general"})
	roomID := created.GetRoom().GetId()

	resp, err := client.ListRoomMembers(outgoingCtxAs("alice-uuid"), &chatv1.ListRoomMembersRequest{RoomId: roomID})
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	members := resp.GetMembers()
	if len(members) != 1 || members[0].GetUserId() != "alice-uuid" {
		t.Fatalf("members = %+v", members)
	}
	if members[0].GetDisplayName() != "" {
		t.Errorf("display_name should be empty when profile missing, got %q", members[0].GetDisplayName())
	}
}

func TestGRPCAdapter_ListRoomMembers_RoomNotFound(t *testing.T) {
	client, _, cleanup := startServer(t)
	defer cleanup()

	_, err := client.ListRoomMembers(outgoingCtxAs("alice"), &chatv1.ListRoomMembersRequest{RoomId: "nonexistent"})
	if code := status.Code(err); code != codes.NotFound {
		t.Errorf("code = %s, want NotFound", code)
	}
}

func TestGRPCAdapter_GetRoom_NotFound(t *testing.T) {
	client, _, cleanup := startServer(t)
	defer cleanup()

	_, err := client.GetRoom(outgoingCtxAs("alice"), &chatv1.GetRoomRequest{RoomId: "nonexistent"})
	if code := status.Code(err); code != codes.NotFound {
		t.Errorf("code = %s, want NotFound", code)
	}
}

func TestGRPCAdapter_LeaveRoom_NotMember(t *testing.T) {
	client, _, cleanup := startServer(t)
	defer cleanup()

	created, _ := client.CreateRoom(outgoingCtxAs("alice"), &chatv1.CreateRoomRequest{Name: "general"})
	_, err := client.LeaveRoom(outgoingCtxAs("bob"), &chatv1.LeaveRoomRequest{RoomId: created.GetRoom().GetId()})
	if code := status.Code(err); code != codes.FailedPrecondition {
		t.Errorf("code = %s, want FailedPrecondition", code)
	}
}

func TestGRPCAdapter_ListRooms_ScopedToCaller(t *testing.T) {
	client, _, cleanup := startServer(t)
	defer cleanup()

	client.CreateRoom(outgoingCtxAs("alice"), &chatv1.CreateRoomRequest{Name: "alice-room"})
	bob, _ := client.CreateRoom(outgoingCtxAs("bob"), &chatv1.CreateRoomRequest{Name: "bob-room"})

	resp, err := client.ListRooms(outgoingCtxAs("bob"), &chatv1.ListRoomsRequest{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.GetRooms()) != 1 || resp.GetRooms()[0].GetId() != bob.GetRoom().GetId() {
		t.Errorf("unexpected rooms for bob: %+v", resp.GetRooms())
	}
}
