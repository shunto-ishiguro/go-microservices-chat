package config

import (
	"fmt"
	"os"
)

// Config は環境変数から読み込むアプリケーション設定
type Config struct {
	Port        string
	DatabaseURL string
	LogLevel    string
}

// Load は環境変数から設定を読み込む（未設定の場合はデフォルト値を使用）
func Load() *Config {
	return &Config{
		Port:        getEnv("PORT", "8001"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://chat:chat@localhost:5432/userdb?sslmode=disable"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
	}
}

// Addr はHTTPサーバー用のアドレス文字列を返す
func (c *Config) Addr() string {
	return fmt.Sprintf(":%s", c.Port)
}

// getEnv は環境変数を取得し、未設定ならフォールバック値を返す
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
