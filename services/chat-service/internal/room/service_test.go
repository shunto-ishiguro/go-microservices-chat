package room_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/metadata"

	authpkg "go-microservices-chat/pkg/auth"
	"go-microservices-chat/services/chat-service/internal/room"
)

func ctxAs(userID string) context.Context {
	return metadata.NewIncomingContext(context.Background(),
		metadata.Pairs(authpkg.MetadataKeyUserID, userID))
}

func TestService_CreateRoom_AutoJoinsCreator(t *testing.T) {
	svc := room.NewService(room.NewInMemRepository())
	r, err := svc.CreateRoom(ctxAs("alice"), "general")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if r.Name != "general" || r.CreatedBy != "alice" {
		t.Errorf("unexpected room: %+v", r)
	}
	if err := svc.EnsureMember(context.Background(), r.ID, "alice"); err != nil {
		t.Errorf("creator is not a member: %v", err)
	}
}

func TestService_CreateRoom_RejectsEmptyName(t *testing.T) {
	svc := room.NewService(room.NewInMemRepository())
	if _, err := svc.CreateRoom(ctxAs("alice"), "  "); !errors.Is(err, room.ErrInvalidArgument) {
		t.Errorf("err = %v, want ErrInvalidArgument", err)
	}
}

func TestService_CreateRoom_RequiresRequester(t *testing.T) {
	svc := room.NewService(room.NewInMemRepository())
	if _, err := svc.CreateRoom(context.Background(), "x"); err == nil {
		t.Error("expected error without requester")
	}
}

func TestService_JoinLeave(t *testing.T) {
	svc := room.NewService(room.NewInMemRepository())
	r, _ := svc.CreateRoom(ctxAs("alice"), "general")

	if err := svc.JoinRoom(ctxAs("bob"), r.ID); err != nil {
		t.Fatalf("join: %v", err)
	}
	// 2 回目の Join は冪等でエラーにならない。
	if err := svc.JoinRoom(ctxAs("bob"), r.ID); err != nil {
		t.Fatalf("join again: %v", err)
	}

	_, count, err := svc.GetRoom(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("member count = %d, want 2", count)
	}

	members, err := svc.ListRoomMembers(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Errorf("members = %v", members)
	}

	if err := svc.LeaveRoom(ctxAs("bob"), r.ID); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if err := svc.LeaveRoom(ctxAs("bob"), r.ID); !errors.Is(err, room.ErrNotMember) {
		t.Errorf("leave twice err = %v", err)
	}
}

func TestService_ListMyRooms_FiltersByMembership(t *testing.T) {
	svc := room.NewService(room.NewInMemRepository())
	svc.CreateRoom(ctxAs("alice"), "alice-room")
	bobRoom, _ := svc.CreateRoom(ctxAs("bob"), "bob-room")

	got, _, err := svc.ListMyRooms(ctxAs("bob"), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != bobRoom.ID {
		t.Errorf("unexpected rooms for bob: %+v", got)
	}
}

func TestService_SearchRooms_MatchesSubstring(t *testing.T) {
	svc := room.NewService(room.NewInMemRepository())
	svc.CreateRoom(ctxAs("alice"), "general")
	svc.CreateRoom(ctxAs("alice"), "random")

	got, _, err := svc.SearchRooms(context.Background(), "gen", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "general" {
		t.Errorf("search result: %+v", got)
	}
}
