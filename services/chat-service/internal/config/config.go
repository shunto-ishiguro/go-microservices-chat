package config

import (
	"fmt"
	"os"
)

// Config は chat-service のランタイム設定 (12-factor で env var から読む)。
//
// 各フィールド:
//
//	GRPCAddr        : gRPC の listen アドレス (デフォルト :50052)
//	DatabaseURL     : chatdb への接続文字列。必須
//	UserServiceAddr : user-service への接続先 (例: "user-service:50051")。
//	                  member enrich で BatchGetUsers を呼ぶために必須
type Config struct {
	GRPCAddr         string
	DatabaseURL      string
	UserServiceAddr  string
}

func Load() (*Config, error) {
	cfg := &Config{
		GRPCAddr:        envOr("GRPC_ADDR", ":50052"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		UserServiceAddr: os.Getenv("USER_SERVICE_ADDR"),
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL is required")
	}
	if cfg.UserServiceAddr == "" {
		return nil, fmt.Errorf("config: USER_SERVICE_ADDR is required")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
