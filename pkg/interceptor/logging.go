package interceptor

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type ctxKey int

const requestIDKey ctxKey = iota

const requestIDHeader = "x-request-id"

// Logging は RPC ごとに構造化 JSON ログを出力し、context にリクエスト ID を
// 注入する unary server interceptor を返す。`authorization` ヘッダは決してログに出さない。
func Logging(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		reqID := extractOrNewRequestID(ctx)
		ctx = context.WithValue(ctx, requestIDKey, reqID)

		resp, err := handler(ctx, req)

		attrs := []slog.Attr{
			slog.String("method", info.FullMethod),
			slog.String("request_id", reqID),
			slog.Duration("duration", time.Since(start)),
		}
		if err != nil {
			attrs = append(attrs,
				slog.String("code", status.Code(err).String()),
				slog.String("error", err.Error()),
			)
			logger.LogAttrs(ctx, slog.LevelError, "rpc failed", attrs...)
		} else {
			attrs = append(attrs, slog.String("code", "OK"))
			logger.LogAttrs(ctx, slog.LevelInfo, "rpc finished", attrs...)
		}
		return resp, err
	}
}

// RequestID は Logging interceptor が context に格納したリクエスト ID を取り出す。
func RequestID(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

// extractOrNewRequestID は incoming metadata から request id を取り出す。
// 上流 (Envoy など) が既に付与していればそれを採用し、無ければ新規発番する。
//
// なぜ上流 ID を優先するか: 分散トレーシングでリクエストを gateway → chat-svc →
// user-svc と追う時、全 hop のログに同じ ID が載っていれば grep 一発で追える。
func extractOrNewRequestID(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get(requestIDHeader); len(vals) > 0 && vals[0] != "" {
			return vals[0]
		}
	}
	return uuid.NewString()
}
