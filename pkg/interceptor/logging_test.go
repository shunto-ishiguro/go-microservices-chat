package interceptor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go-microservices-chat/pkg/interceptor"
)

func newLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logger, buf
}

func TestLogging_SuccessEmitsJSON(t *testing.T) {
	logger, buf := newLogger()
	var seenReqID string
	handler := func(ctx context.Context, req any) (any, error) {
		seenReqID = interceptor.RequestID(ctx)
		return "ok", nil
	}

	resp, err := interceptor.Logging(logger)(
		context.Background(),
		"req",
		&grpc.UnaryServerInfo{FullMethod: "/user.v1.UserService/Register"},
		handler,
	)
	if err != nil || resp != "ok" {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
	if seenReqID == "" {
		t.Fatal("request id not injected")
	}

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log is not JSON: %v\n%s", err, buf.String())
	}
	if entry["code"] != "OK" {
		t.Errorf("code = %v", entry["code"])
	}
	if entry["method"] != "/user.v1.UserService/Register" {
		t.Errorf("method = %v", entry["method"])
	}
	if entry["request_id"] != seenReqID {
		t.Errorf("request_id mismatch: %v vs %v", entry["request_id"], seenReqID)
	}
}

func TestLogging_PropagatesIncomingRequestID(t *testing.T) {
	logger, _ := newLogger()
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("x-request-id", "req-123"))

	var seen string
	_, _ = interceptor.Logging(logger)(
		ctx, "req",
		&grpc.UnaryServerInfo{FullMethod: "/x"},
		func(ctx context.Context, _ any) (any, error) {
			seen = interceptor.RequestID(ctx)
			return nil, nil
		},
	)
	if seen != "req-123" {
		t.Errorf("request id = %q, want req-123", seen)
	}
}

func TestLogging_ErrorLogsCode(t *testing.T) {
	logger, buf := newLogger()
	wantErr := status.Error(codes.PermissionDenied, "nope")
	_, err := interceptor.Logging(logger)(
		context.Background(), "req",
		&grpc.UnaryServerInfo{FullMethod: "/x"},
		func(ctx context.Context, _ any) (any, error) { return nil, wantErr },
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v", err)
	}
	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log not JSON: %v", err)
	}
	if entry["code"] != "PermissionDenied" {
		t.Errorf("code = %v", entry["code"])
	}
}
