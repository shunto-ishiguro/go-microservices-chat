package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"go-microservices-chat/pkg/logger"
	"go-microservices-chat/services/user-service/internal/config"
	"go-microservices-chat/services/user-service/internal/handler/rest"
	"go-microservices-chat/services/user-service/internal/repository"
	"go-microservices-chat/services/user-service/internal/service"
)

func main() {
	cfg := config.Load()
	log := logger.New(cfg.LogLevel)

	// PostgreSQL に接続
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Error("データベース接続に失敗", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	// 接続確認（ping）
	if err := pool.Ping(context.Background()); err != nil {
		log.Error("データベースへの疎通確認に失敗", slog.String("error", err.Error()))
		os.Exit(1)
	}
	log.Info("データベースに接続しました")

	// 各層を組み立てる（Repository → Service → Handler）
	repo := repository.NewPostgresUserRepository(pool)
	svc := service.NewUserService(repo)
	router := rest.NewRouter(svc, log)

	// HTTPサーバーの設定
	srv := &http.Server{
		Addr:         cfg.Addr(),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// サーバーを別のgoroutineで起動
	go func() {
		log.Info("サーバーを起動します", slog.String("addr", cfg.Addr()))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("サーバーエラー", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// Graceful Shutdown: SIGINT（Ctrl+C）または SIGTERM を受け取ったら安全に停止
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Info("シャットダウンを開始します", slog.String("signal", sig.String()))

	// 処理中のリクエストが完了するまで最大10秒待つ
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("サーバーの強制停止", slog.String("error", err.Error()))
		os.Exit(1)
	}

	log.Info("サーバーを停止しました")
}
