// Package chatclient は realtime-service が chat-service に永続化を依頼する gRPC クライアント。
//
// 「永続化と配信を分離する」設計の永続化側エンドポイント。受信した WS メッセージは:
//  1. chatclient.SendMessage で chat-service に書く (この package の責務)
//  2. pubsub.Publish で Redis に流す (別 package、別 goroutine で並行に走らせる)
//
// realtime-service は chat-service を「呼ぶ側」なので gRPC Server は持たず、Client のみ。
// 呼び出し時は x-user-id を outgoing metadata に詰めて chat-service の認証に使わせる
// (Envoy 経由ではなく直接 gRPC なので、メタデータ伝搬の責任が realtime-service 側にある)。
package chatclient

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	chatv1 "go-microservices-chat/gen/go/chat/v1"
	"go-microservices-chat/pkg/auth"
)

// Client は chat-service の薄いラッパ。テストでは fake.go の Fake に差し替える。
type Client interface {
	// SendMessage は永続化依頼。Realtime path では fire-and-forget で良いので、
	// 戻り値の Message までは返さず errror のみ通知する設計にしている。
	SendMessage(ctx context.Context, senderID, roomID, content string) error
	Close() error
}

type grpcClient struct {
	conn *grpc.ClientConn
	svc  chatv1.ChatServiceClient
}

func Dial(addr string) (Client, error) {
	if addr == "" {
		return nil, fmt.Errorf("chatclient: addr is required")
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("chatclient: dial: %w", err)
	}
	return &grpcClient{conn: conn, svc: chatv1.NewChatServiceClient(conn)}, nil
}

func (c *grpcClient) SendMessage(ctx context.Context, senderID, roomID, content string) error {
	// chat-service は x-user-id metadata で「呼び出し元」を確定する。realtime-service は
	// Envoy を経由しない直接 gRPC なので、ここで明示的に metadata を載せる必要がある。
	ctx = metadata.AppendToOutgoingContext(ctx, auth.MetadataKeyUserID, senderID)
	_, err := c.svc.SendMessage(ctx, &chatv1.SendMessageRequest{
		RoomId:   roomID,
		SenderId: senderID,
		Content:  content,
	})
	return err
}

func (c *grpcClient) Close() error { return c.conn.Close() }
