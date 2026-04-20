package user_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"

	authpkg "go-microservices-chat/pkg/auth"
	"go-microservices-chat/services/user-service/internal/user"
)

type fakeIssuer struct{ counter int }

func (f *fakeIssuer) IssueAccessToken(userID, _ string) (string, error) {
	f.counter++
	return "access-token-" + userID, nil
}

func newService() *user.Service {
	return user.NewService(user.NewInMemRepository(), &fakeIssuer{})
}

func ctxAs(userID string) context.Context {
	return metadata.NewIncomingContext(context.Background(),
		metadata.Pairs(authpkg.MetadataKeyUserID, userID))
}

func TestService_RegisterAndLogin(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	u, err := svc.Register(ctx, "Alice@Example.com", "alice", "Alice", "pw12345")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if u.Email != "alice@example.com" {
		t.Errorf("email not normalized: %q", u.Email)
	}
	if u.PasswordHash == "" || u.PasswordHash == "pw12345" {
		t.Errorf("password not hashed: %q", u.PasswordHash)
	}

	access, refresh, err := svc.Login(ctx, "alice@example.com", "pw12345")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if access == "" || refresh == "" {
		t.Fatalf("empty tokens: access=%q refresh=%q", access, refresh)
	}
}

func TestService_Register_Duplicate(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	if _, err := svc.Register(ctx, "a@b.com", "alice", "Alice", "pw"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Register(ctx, "a@b.com", "bob", "Bob", "pw")
	if err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestService_Login_WrongPassword(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	if _, err := svc.Register(ctx, "a@b.com", "alice", "Alice", "pw12345"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Login(ctx, "a@b.com", "wrong"); err == nil {
		t.Fatal("expected invalid credentials")
	}
}

func TestService_Refresh_Rotates(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	u, _ := svc.Register(ctx, "a@b.com", "alice", "Alice", "pw12345")
	_, r1, _ := svc.Login(ctx, "a@b.com", "pw12345")

	a2, r2, err := svc.Refresh(ctx, r1)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if r2 == r1 {
		t.Fatal("refresh token was not rotated")
	}
	if a2 == "" || a2 != "access-token-"+u.ID {
		t.Errorf("unexpected access token: %q", a2)
	}

	if _, _, err := svc.Refresh(ctx, r1); err == nil {
		t.Fatal("old refresh should be rejected")
	}
}

func TestService_UpdateMe_UpdatesSelfOnly(t *testing.T) {
	svc := newService()
	u, _ := svc.Register(context.Background(), "a@b.com", "alice", "Alice", "pw12345")

	name := "Alice Alt"
	updated, err := svc.UpdateMe(ctxAs(u.ID), &name, nil, nil)
	if err != nil {
		t.Fatalf("update me: %v", err)
	}
	if updated.DisplayName != "Alice Alt" {
		t.Errorf("display_name = %q", updated.DisplayName)
	}
	if updated.ID != u.ID {
		t.Errorf("updated ID = %q, want %q (self only)", updated.ID, u.ID)
	}

	if _, err := svc.UpdateMe(context.Background(), &name, nil, nil); err == nil {
		t.Fatal("no requester should be rejected")
	}
}

func TestService_BatchGetUsers(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	alice, _ := svc.Register(ctx, "a@b.com", "alice", "Alice", "pw12345")
	bob, _ := svc.Register(ctx, "b@b.com", "bob", "Bob", "pw12345")

	got, err := svc.BatchGetUsers(ctx, []string{alice.ID, bob.ID, "missing-id"})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (missing-id は欠落)", len(got))
	}
	ids := map[string]bool{}
	for _, u := range got {
		ids[u.ID] = true
	}
	if !ids[alice.ID] || !ids[bob.ID] {
		t.Errorf("missing users: %+v", ids)
	}
}

func TestService_GetMe_ReturnsRequester(t *testing.T) {
	svc := newService()
	u, _ := svc.Register(context.Background(), "a@b.com", "alice", "Alice", "pw12345")

	got, err := svc.GetMe(ctxAs(u.ID))
	if err != nil {
		t.Fatalf("get me: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("id = %q, want %q", got.ID, u.ID)
	}

	if _, err := svc.GetMe(context.Background()); err == nil {
		t.Fatal("no requester should be rejected")
	}
}
