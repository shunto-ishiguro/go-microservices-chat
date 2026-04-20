// Package main は user-service のエントリーポイント。
//
// 全体フロー:
//  1. env var から config を読み込む
//  2. RSA 秘密鍵をパース (JWT 発行用)
//  3. pgxpool で PostgreSQL 接続プールを張る
//  4. Repository → Service → GRPCAdapter の順で DI 組み立て
//  5. 2 つのサーバーを別 goroutine で同時起動:
//       - gRPC :50051 (本体 API)
//       - JWKS HTTP :8082 (公開鍵配信、infra 側 Envoy が起動時に取りに来る)
//  6. SIGTERM / SIGINT を受けると両方 graceful shutdown
package main

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"

	userv1 "go-microservices-chat/gen/go/user/v1"
	"go-microservices-chat/pkg/auth"
	"go-microservices-chat/pkg/interceptor"
	"go-microservices-chat/services/user-service/internal/config"
	"go-microservices-chat/services/user-service/internal/user"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("user-service exited with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	priv, err := parseRSAPrivateKey(cfg.JWTPrivateKey)
	if err != nil {
		return fmt.Errorf("parse private key: %w", err)
	}

	// SIGTERM / SIGINT を受け取ると ctx が Done になり、shutdown へ進む。
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	// DI: 下から上に組み立てる。Repository は interface なのでテストでは InMem に差し替えられる。
	repo := user.NewPostgresRepository(pool)
	issuer := auth.NewIssuer(priv, cfg.JWTKeyID)
	svc := user.NewService(repo, issuer)

	grpcSrv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(interceptor.Logging(logger)),
	)
	userv1.RegisterUserServiceServer(grpcSrv, user.NewGRPCAdapter(svc))

	grpcLis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("listen grpc: %w", err)
	}

	// JWKS エンドポイントは gRPC とは別ポート。Envoy は起動時 1 回だけここを fetch して
	// 公開鍵をキャッシュする (以降は JWT 検証をオフラインで実行できる)。
	jwksMux := http.NewServeMux()
	jwksMux.Handle("/.well-known/jwks.json", auth.NewJWKSHandler(&priv.PublicKey, cfg.JWTKeyID))
	jwksSrv := &http.Server{
		Addr:              cfg.JWKSAddr,
		Handler:           jwksMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// 2 つのサーバーを goroutine で並行起動し、どちらかが死んだら errCh 経由で main に戻す。
	errCh := make(chan error, 2)
	go func() {
		logger.Info("grpc listening", "addr", cfg.GRPCAddr)
		errCh <- grpcSrv.Serve(grpcLis)
	}()
	go func() {
		logger.Info("jwks listening", "addr", cfg.JWKSAddr)
		if err := jwksSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	// ① シグナルで ctx.Done、または ② どちらかのサーバーがエラー終了で errCh に値が入る。
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			return err
		}
	}

	// Graceful shutdown: 処理中のリクエストを捌き切ってから閉じる。
	// 5 秒でタイムアウトさせ、それ以上は強制終了に倒す (K8s の terminationGracePeriodSeconds と揃える想定)。
	grpcSrv.GracefulStop()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = jwksSrv.Shutdown(shutdownCtx)
	return nil
}

func parseRSAPrivateKey(pem string) (*rsa.PrivateKey, error) {
	return jwt.ParseRSAPrivateKeyFromPEM([]byte(pem))
}
