package logger

import (
	"log/slog"
	"os"
)

// New は設定済みの slog.Logger を生成する
// level: "debug", "info", "warn", "error"（デフォルトは "info"）
func New(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	// JSON形式で標準出力にログを出す
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: lvl,
	})

	return slog.New(handler)
}
