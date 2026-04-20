package user_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	userv1 "go-microservices-chat/gen/go/user/v1"
	"go-microservices-chat/pkg/auth"
	"go-microservices-chat/services/user-service/internal/user"
)

func startBufconnServer(t *testing.T) (userv1.UserServiceClient, func()) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa: %v", err)
	}
	repo := user.NewInMemRepository()
	issuer := auth.NewIssuer(priv, "test-key")
	svc := user.NewService(repo, issuer)

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	userv1.RegisterUserServiceServer(srv, user.NewGRPCAdapter(svc))
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
		_ = conn.Close()
		srv.Stop()
	}
	return userv1.NewUserServiceClient(conn), cleanup
}

func TestGRPCAdapter_RegisterLoginGetMeUpdateMe(t *testing.T) {
	client, cleanup := startBufconnServer(t)
	defer cleanup()

	ctx := context.Background()
	reg, err := client.Register(ctx, &userv1.RegisterRequest{
		Email: "alice@example.com", Username: "alice", DisplayName: "Alice", Password: "pw12345",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	aliceID := reg.GetUser().GetId()
	if aliceID == "" {
		t.Fatal("no user id returned")
	}

	login, err := client.Login(ctx, &userv1.LoginRequest{Email: "alice@example.com", Password: "pw12345"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if login.GetAccessToken() == "" {
		t.Fatal("no access token")
	}

	ctxAlice := metadata.NewOutgoingContext(ctx, metadata.Pairs(auth.MetadataKeyUserID, aliceID))
	me, err := client.GetMe(ctxAlice, &userv1.GetMeRequest{})
	if err != nil {
		t.Fatalf("get me: %v", err)
	}
	if me.GetUser().GetEmail() != "alice@example.com" {
		t.Errorf("email = %q", me.GetUser().GetEmail())
	}

	newName := "Alice Updated"
	upd, err := client.UpdateMe(ctxAlice, &userv1.UpdateMeRequest{DisplayName: &newName})
	if err != nil {
		t.Fatalf("update me: %v", err)
	}
	if upd.GetUser().GetDisplayName() != "Alice Updated" {
		t.Errorf("display_name = %q", upd.GetUser().GetDisplayName())
	}
	if upd.GetUser().GetId() != aliceID {
		t.Errorf("updated ID = %q, want %q (self only)", upd.GetUser().GetId(), aliceID)
	}
}

func TestGRPCAdapter_GetMe_Unauthenticated(t *testing.T) {
	client, cleanup := startBufconnServer(t)
	defer cleanup()

	// x-user-id が無いと Unauthenticated になる (ゲートウェイ未通過の状態)。
	_, err := client.GetMe(context.Background(), &userv1.GetMeRequest{})
	if code := status.Code(err); code != codes.Unauthenticated {
		t.Errorf("code = %s, want Unauthenticated", code)
	}
}

func TestGRPCAdapter_GetUser_InternalLookup(t *testing.T) {
	client, cleanup := startBufconnServer(t)
	defer cleanup()

	ctx := context.Background()
	reg, _ := client.Register(ctx, &userv1.RegisterRequest{
		Email: "alice@example.com", Username: "alice", DisplayName: "Alice", Password: "pw12345",
	})
	aliceID := reg.GetUser().GetId()

	// GetUser は内部 RPC。member enrich 用途で他人の情報を引く (呼び出し元は bob だが alice を取る)。
	bobCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs(auth.MetadataKeyUserID, "bob-uuid"))
	got, err := client.GetUser(bobCtx, &userv1.GetUserRequest{UserId: aliceID})
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.GetUser().GetUsername() != "alice" {
		t.Errorf("username = %q", got.GetUser().GetUsername())
	}
}

func TestGRPCAdapter_Register_AlreadyExists(t *testing.T) {
	client, cleanup := startBufconnServer(t)
	defer cleanup()

	ctx := context.Background()
	req := &userv1.RegisterRequest{Email: "a@b.com", Username: "alice", DisplayName: "Alice", Password: "pw"}
	if _, err := client.Register(ctx, req); err != nil {
		t.Fatal(err)
	}
	_, err := client.Register(ctx, req)
	if code := status.Code(err); code != codes.AlreadyExists {
		t.Errorf("code = %s, want AlreadyExists", code)
	}
}
