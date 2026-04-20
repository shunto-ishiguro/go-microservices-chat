package auth

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
)

// JWKSHandler は `/.well-known/jwks.json` で設定された公開鍵を JWKS 形式で配信する。
// ゲートウェイ (Envoy) がこれを取得して、Issuer が署名した JWT を検証する。
type JWKSHandler struct {
	publicKey *rsa.PublicKey
	keyID     string
}

func NewJWKSHandler(publicKey *rsa.PublicKey, keyID string) *JWKSHandler {
	return &JWKSHandler{publicKey: publicKey, keyID: keyID}
}

// ServeHTTP は JWKS (RFC 7517) の JSON を 1 件返す。
//
// フィールドの意味:
//
//	kty : キータイプ (RSA 固定)
//	kid : key ID。JWT ヘッダの `kid` と一致する鍵を検証側が引くためのキー
//	alg : 署名アルゴリズム (RS256)
//	use : 用途 ("sig" = signature)
//	n   : RSA modulus (base64url、先頭ゼロパディングなし)
//	e   : RSA exponent (同上)
//
// Cache-Control は 300 秒 (5 分)。Envoy は起動時 1 回 fetch して永続的にキャッシュし、
// 鍵ローテーション時は新旧 2 つを JWKS に並べる運用になる (今回は 1 つだけ)。
func (h *JWKSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	jwks := map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"kid": h.keyID,
				"alg": "RS256",
				"use": "sig",
				"n":   base64url(h.publicKey.N.Bytes()),
				"e":   base64url(encodeExponent(h.publicKey.E)),
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_ = json.NewEncoder(w).Encode(jwks)
}

// base64url は JWKS の仕様 (RFC 7515 §2) に沿って base64url (パディング無し) でエンコードする。
// 標準 base64 と違って URL safe かつ '=' を付けない形式。
func base64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// encodeExponent は e の big-endian バイト表現から先頭のゼロバイトを除いて返す
// (JWKS RFC 7518 は最小表現を要求している)。
func encodeExponent(e int) []byte {
	buf := []byte{
		byte(e >> 24),
		byte(e >> 16),
		byte(e >> 8),
		byte(e),
	}
	for len(buf) > 1 && buf[0] == 0 {
		buf = buf[1:]
	}
	return buf
}
