// Package main は realtime-service のエントリーポイント。
//
// 配線:
//  1. config から REDIS_ADDR / CHAT_SERVICE_ADDR を読む
//  2. Redis に接続して Ping を打つ (起動時に死活確認)
//  3. chat-service への gRPC クライアントを Dial (永続化経路)
//  4. Hub を起動し、Subscriber goroutine で Redis → Hub.LocalBroadcast に流す
//  5. WebSocket Handler を /ws にマウントして HTTP サーバーを起動
//  6. SIGTERM / SIGINT で graceful shutdown (HTTP → Subscriber → Redis → chat-svc の順で閉じる)
//
// 本サービスは JWT 検証しない (= Envoy の責務)。X-User-Id ヘッダを信じて読むだけ。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-microservices-chat/services/realtime-service/internal/chatclient"
	"go-microservices-chat/services/realtime-service/internal/config"
	"go-microservices-chat/services/realtime-service/internal/hub"
	"go-microservices-chat/services/realtime-service/internal/pubsub"
	"go-microservices-chat/services/realtime-service/internal/ws"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("realtime-service exited with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// SIGTERM / SIGINT で ctx.Done。ws ハンドラ含め一斉に解放される。
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	// Redis Pub/Sub。起動時に Ping で死活確認 (Redis 落ちなら起動を断念)。
	ps := pubsub.NewRedis(cfg.RedisAddr)
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := ps.Ping(pingCtx); err != nil {
		pingCancel()
		return fmt.Errorf("ping redis: %w", err)
	}
	pingCancel()
	defer ps.Close()

	// chat-service への gRPC クライアント。プロセス寿命と同じ長寿命接続を 1 本だけ持つ。
	cc, err := chatclient.Dial(cfg.ChatServiceAddr)
	if err != nil {
		return fmt.Errorf("dial chat-service: %w", err)
	}
	defer cc.Close()

	// Hub: WebSocket クライアント集合の管理。1 goroutine で直列処理。
	h := hub.NewHub()
	go h.Run()
	defer h.Stop()

	// Subscriber: Redis から流れてくるイベントを Hub.LocalBroadcast に流す。
	// プロセス寿命と同じ goroutine。エラーは log だけ残す (再接続は Phase 4 以降)。
	go func() {
		err := ps.Subscribe(ctx, func(ev pubsub.RoomEvent) {
			h.LocalBroadcast(hub.LocalEvent{RoomID: ev.RoomID, Payload: ev.Payload})
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("pubsub subscribe ended", "error", err)
		}
	}()

	// HTTP server: /ws のみ。標準の net/http で十分 (Phase 2 では rate limit 等は infra 責務)。
	mux := http.NewServeMux()
	mux.Handle("/ws", ws.NewHandler(logger, h, ps, cc))
	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http listening", "addr", cfg.HTTPAddr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	// HTTP を graceful に閉じる (open WebSocket は Shutdown ではすぐには閉じないが、
	// ctx.Done で各 handler の readLoop が抜ける構成にしてあるので連動して終わる)。
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	return srv.Shutdown(shutdownCtx)
}
