package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

const RequestIDHeader = "X-Request-ID"

// RequestID は各リクエストにユニークなIDを付与するミドルウェア
// クライアントが X-Request-ID ヘッダーを送ってきた場合はそれを使い、
// なければ新しいUUIDを生成する
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if id == "" {
			id = uuid.New().String()
		}

		// リクエストIDをコンテキストに保存し、レスポンスヘッダーにも付与
		ctx := context.WithValue(r.Context(), RequestIDKey, id)
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID はコンテキストからリクエストIDを取得する
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}
