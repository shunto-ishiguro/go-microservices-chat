package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-microservices-chat/pkg/auth"
)

func TestJWKSHandler_ServesKey(t *testing.T) {
	priv := mustKey(t)
	h := auth.NewJWKSHandler(&priv.PublicKey, "k1")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}

	var body struct {
		Keys []map[string]string `json:"keys"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Keys) != 1 {
		t.Fatalf("len(keys) = %d", len(body.Keys))
	}
	key := body.Keys[0]
	for _, field := range []string{"kty", "kid", "alg", "use", "n", "e"} {
		if key[field] == "" {
			t.Errorf("missing field %q: %v", field, key)
		}
	}
	if key["kty"] != "RSA" {
		t.Errorf("kty = %q", key["kty"])
	}
	if key["kid"] != "k1" {
		t.Errorf("kid = %q", key["kid"])
	}
	if key["alg"] != "RS256" {
		t.Errorf("alg = %q", key["alg"])
	}
}
