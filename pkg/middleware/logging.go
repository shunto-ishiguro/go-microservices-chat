package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// wrappedWriter は ResponseWriter をラップしてステータスコードを記録する
type wrappedWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *wrappedWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// Logging は各リクエストのメソッド・パス・ステータス・所要時間をログ出力するミドルウェア
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			wrapped := &wrappedWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrapped, r)

			logger.Info("request completed",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", wrapped.statusCode),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", GetRequestID(r.Context())),
			)
		})
	}
}
