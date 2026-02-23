package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recovery はパニック（予期しないクラッシュ）から復帰してサーバーを落とさずに500を返すミドルウェア
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("パニックから復帰",
						slog.Any("error", err),
						slog.String("stack", string(debug.Stack())),
						slog.String("request_id", GetRequestID(r.Context())),
					)
					http.Error(w, `{"error":{"code":"INTERNAL_ERROR","message":"an internal error occurred"}}`, http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
