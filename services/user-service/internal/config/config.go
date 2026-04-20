package config

import (
	"fmt"
	"os"
)

// Config は user-service のランタイム設定 (12-factor で env var から読む)。
// dev 用の値は Phase 4 の compose.yaml で注入する。
//
// 各フィールド:
//
//	GRPCAddr       : gRPC の listen アドレス (デフォルト :50051)
//	JWKSAddr       : JWKS HTTP サーバーの listen アドレス (デフォルト :8082)。Envoy が起動時に取りに来る
//	DatabaseURL    : PostgreSQL 接続文字列。必須
//	JWTPrivateKey  : RSA 秘密鍵 PEM。必須。inline (JWT_PRIVATE_KEY) or ファイル (JWT_PRIVATE_KEY_FILE)
//	JWTKeyID       : JWT ヘッダの `kid`。JWKS 側の key id と一致させる。鍵ローテーション時に新旧を区別
type Config struct {
	GRPCAddr       string
	JWKSAddr       string
	DatabaseURL    string
	JWTPrivateKey  string
	JWTKeyID       string
}

func Load() (*Config, error) {
	cfg := &Config{
		GRPCAddr:      envOr("GRPC_ADDR", ":50051"),
		JWKSAddr:      envOr("JWKS_ADDR", ":8082"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		JWTPrivateKey: loadPrivateKeyMaterial(),
		JWTKeyID:      envOr("JWT_KEY_ID", "user-service-key"),
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL is required")
	}
	if cfg.JWTPrivateKey == "" {
		return nil, fmt.Errorf("config: JWT_PRIVATE_KEY or JWT_PRIVATE_KEY_FILE is required")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadPrivateKeyMaterial は RSA 秘密鍵を読み込む。JWT_PRIVATE_KEY (PEM 文字列) か
// JWT_PRIVATE_KEY_FILE (ファイルパス) のいずれかを受け付ける。
func loadPrivateKeyMaterial() string {
	if v := os.Getenv("JWT_PRIVATE_KEY"); v != "" {
		return v
	}
	if p := os.Getenv("JWT_PRIVATE_KEY_FILE"); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			return ""
		}
		return string(b)
	}
	return ""
}
