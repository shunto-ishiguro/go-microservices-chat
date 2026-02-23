package rest

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"go-microservices-chat/pkg/middleware"
	"go-microservices-chat/services/user-service/internal/service"
)

// NewRouter は全ルートとミドルウェアを設定した chi ルーターを生成する
func NewRouter(svc *service.UserService, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()

	// 全リクエストに適用するミドルウェア
	r.Use(middleware.RequestID)
	r.Use(middleware.Logging(logger))
	r.Use(middleware.Recovery(logger))

	userHandler := NewUserHandler(svc)

	// ヘルスチェック
	r.Get("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// ユーザー関連ルート
	r.Route("/api/v1/users", func(r chi.Router) {
		r.Post("/", userHandler.Create)
		r.Get("/", userHandler.List)
		r.Get("/{id}", userHandler.Get)
		r.Put("/{id}", userHandler.Update)
		r.Delete("/{id}", userHandler.Delete)
	})

	return r
}
