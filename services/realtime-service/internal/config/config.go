// Package config は realtime-service の env var 設定。
//
// 必須:
//
//	REDIS_ADDR        : Redis Pub/Sub の接続先 (例 "localhost:6379")
//	CHAT_SERVICE_ADDR : chat-service の gRPC 接続先 (例 "localhost:50052")
//
// 任意:
//
//	HTTP_ADDR : WebSocket の listen アドレス (デフォルト ":8081")
package config

import (
	"fmt"
	"os"
)

type Config struct {
	HTTPAddr        string
	RedisAddr       string
	ChatServiceAddr string
}

func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:        envOr("HTTP_ADDR", ":8081"),
		RedisAddr:       os.Getenv("REDIS_ADDR"),
		ChatServiceAddr: os.Getenv("CHAT_SERVICE_ADDR"),
	}
	if cfg.RedisAddr == "" {
		return nil, fmt.Errorf("config: REDIS_ADDR is required")
	}
	if cfg.ChatServiceAddr == "" {
		return nil, fmt.Errorf("config: CHAT_SERVICE_ADDR is required")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
