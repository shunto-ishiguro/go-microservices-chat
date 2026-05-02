// Package main は chat-service のエントリーポイント。
//
// 全体フロー:
//  1. env var から config を読み込む (DATABASE_URL / USER_SERVICE_ADDR)
//  2. pgxpool で chatdb への接続プールを張る
//  3. user-service への gRPC クライアントを張る (長寿命接続、member enrich 用)
//  4. room.Repository → room.Service → room.GRPCAdapter の順で DI
//  5. gRPC :50052 を起動し、SIGTERM / SIGINT で graceful shutdown
//
// 本サービスは JWT 検証しない。Envoy から届く x-user-id メタデータを信じて読むだけ。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"

	chatv1 "go-microservices-chat/gen/go/chat/v1"
	"go-microservices-chat/pkg/interceptor"
	"go-microservices-chat/services/chat-service/internal/config"
	chatgrpc "go-microservices-chat/services/chat-service/internal/grpc"
	"go-microservices-chat/services/chat-service/internal/message"
	"go-microservices-chat/services/chat-service/internal/room"
	"go-microservices-chat/services/chat-service/internal/userclient"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("chat-service exited with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// SIGTERM / SIGINT で ctx が Done になり graceful shutdown へ。
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	// user-service への gRPC 接続はプロセス寿命と同じ長寿命。
	// 毎リクエストごとに Dial するのは高コストなので 1 本保持する。
	uc, err := userclient.Dial(cfg.UserServiceAddr)
	if err != nil {
		return fmt.Errorf("dial user-service: %w", err)
	}
	defer uc.Close()

	// DI: Repository → Service → GRPCAdapter の順に注入。interface 経由なのでテストでは
	// InMem Repository + fake userclient に差し替えられる。
	// Phase 2 から Room/Message の Adapter を分け、internal/grpc.Server で合流させる。
	roomSvc := room.NewService(room.NewPostgresRepository(pool))
	messageSvc := message.NewService(message.NewPostgresRepository(pool))
	server := chatgrpc.NewServer(
		room.NewGRPCAdapter(roomSvc, uc),
		message.NewGRPCAdapter(messageSvc, roomSvc),
	)
	grpcSrv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(interceptor.Logging(logger)),
	)
	chatv1.RegisterChatServiceServer(grpcSrv, server)

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("grpc listening", "addr", cfg.GRPCAddr)
		errCh <- grpcSrv.Serve(lis)
	}()

	// ① シグナル受信で ctx.Done、または ② サーバー異常終了で errCh に値が入る。
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			return err
		}
	}

	// 処理中リクエストを捌き切ってから終了。
	grpcSrv.GracefulStop()
	return nil
}
