package auth

import (
	"crypto/rsa"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// defaultIssuer: JWT の `iss` クレーム。検証側 (Envoy) の SecurityPolicy でこの値を期待値として設定する。
	defaultIssuer = "chat-app"
	// accessTokenTTL: Access token の有効期限 15 分。短命にする理由は漏洩時のリスク窓を狭めるため。
	// クライアントは失効前にリフレッシュトークンで更新する。
	accessTokenTTL = 15 * time.Minute
)

// Claims はこのサービスが発行するアクセストークンに埋め込むペイロード。
type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Issuer は RSA 秘密鍵で JWT に署名する。
type Issuer struct {
	privateKey *rsa.PrivateKey
	keyID      string
	issuer     string
	now        func() time.Time
}

// NewIssuer は Issuer を生成する。keyID は JWT ヘッダの `kid` として載せ、
// 検証側 (Envoy) が JWKS エンドポイントから対応する公開鍵を引けるようにする。
func NewIssuer(privateKey *rsa.PrivateKey, keyID string) *Issuer {
	return &Issuer{
		privateKey: privateKey,
		keyID:      keyID,
		issuer:     defaultIssuer,
		now:        time.Now,
	}
}

// IssueAccessToken は指定した subject で RS256 署名済みの JWT を返す。
//
// ヘッダの `kid` は JWKS エンドポイントで配信する公開鍵のキー ID と一致する。
// 検証側はこの `kid` を使って JWKS から該当の公開鍵を引き、署名を検証する (鍵ローテーション前提)。
func (i *Issuer) IssueAccessToken(userID, username string) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("auth: userID is required")
	}
	now := i.now()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.issuer,
			Subject:   userID, // Envoy はこの sub を x-user-id として app サービスに注入する
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTokenTTL)),
		},
	})
	token.Header["kid"] = i.keyID
	signed, err := token.SignedString(i.privateKey)
	if err != nil {
		return "", fmt.Errorf("auth: sign access token: %w", err)
	}
	return signed, nil
}
