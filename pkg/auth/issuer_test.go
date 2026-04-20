package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"go-microservices-chat/pkg/auth"
)

func mustKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func TestIssuer_IssueAccessToken(t *testing.T) {
	priv := mustKey(t)
	issuer := auth.NewIssuer(priv, "test-key")

	signed, err := issuer.IssueAccessToken("alice-uuid", "alice")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	parsed, err := jwt.ParseWithClaims(signed, &auth.Claims{}, func(t *jwt.Token) (any, error) {
		return &priv.PublicKey, nil
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if got := parsed.Method.Alg(); got != "RS256" {
		t.Errorf("alg = %q, want RS256", got)
	}
	if got := parsed.Header["kid"]; got != "test-key" {
		t.Errorf("kid = %v, want test-key", got)
	}

	claims, ok := parsed.Claims.(*auth.Claims)
	if !ok {
		t.Fatalf("claims type = %T", parsed.Claims)
	}
	if claims.UserID != "alice-uuid" {
		t.Errorf("user_id = %q", claims.UserID)
	}
	if claims.Username != "alice" {
		t.Errorf("username = %q", claims.Username)
	}
	if claims.Issuer != "chat-app" {
		t.Errorf("iss = %q", claims.Issuer)
	}
	if claims.Subject != "alice-uuid" {
		t.Errorf("sub = %q", claims.Subject)
	}
	if claims.ExpiresAt == nil || time.Until(claims.ExpiresAt.Time) <= 0 {
		t.Errorf("exp not in future: %+v", claims.ExpiresAt)
	}
}

func TestIssuer_IssueAccessToken_EmptyUserID(t *testing.T) {
	issuer := auth.NewIssuer(mustKey(t), "k1")
	if _, err := issuer.IssueAccessToken("", "alice"); err == nil {
		t.Fatal("expected error for empty user id")
	}
}
