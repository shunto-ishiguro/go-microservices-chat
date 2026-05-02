package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"go-microservices-chat/pkg/auth"
	"go-microservices-chat/services/realtime-service/internal/chatclient"
	"go-microservices-chat/services/realtime-service/internal/hub"
	"go-microservices-chat/services/realtime-service/internal/pubsub"
)

// Handler は HTTP /ws の Upgrade を担当し、1 接続あたり read/write 2 つの goroutine を立てる。
//
// 認証方針: アプリ側は JWT を検証しない (= Envoy の責務)。
// Envoy が X-User-Id ヘッダ (または `x-user-id` query param、どちらか先に来た方) で
// 認証済みユーザー ID を注入するので、それを信じて読むだけ。
//
// 1 接続 = 1 room の制約: 接続時に query parameter `room_id` を必須にする。
// 複数 room を 1 接続で扱いたくなったら ws.Inbound に "join" 型を追加する形で拡張する。
type Handler struct {
	logger     *slog.Logger
	hub        *hub.Hub
	pubsub     pubsub.Publisher
	chatClient chatclient.Client
	now        func() time.Time
}

func NewHandler(logger *slog.Logger, h *hub.Hub, ps pubsub.Publisher, cc chatclient.Client) *Handler {
	return &Handler{logger: logger, hub: h, pubsub: ps, chatClient: cc, now: time.Now}
}

// ServeHTTP は HTTP Upgrade のエントリーポイント。
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID := readUserID(r)
	roomID := r.URL.Query().Get("room_id")
	if userID == "" {
		http.Error(w, "missing x-user-id", http.StatusUnauthorized)
		return
	}
	if roomID == "" {
		http.Error(w, "missing room_id", http.StatusBadRequest)
		return
	}

	// dev では Origin チェック無効化 (Envoy 経由なら Origin は信用できる)。
	// 本番で必要になったら infra 側 Envoy か OriginPatterns で絞る。
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		h.logger.Error("ws: accept failed", "error", err)
		return
	}
	defer conn.Close(websocket.StatusInternalError, "internal error")

	client := hub.NewClient(userID, roomID, 32)
	h.hub.Register(client)
	defer h.hub.Unregister(client)

	// 切断条件のいずれかで ctx を Done にして他方の goroutine も止める。
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// write goroutine: hub から流れてきた payload を WS に書き出す。
	go h.writeLoop(ctx, conn, client)

	// read goroutine (= 本 goroutine): クライアントからの message を捌く。
	h.readLoop(ctx, conn, userID, roomID)
	conn.Close(websocket.StatusNormalClosure, "")
}

// readUserID は Envoy が注入する X-User-Id ヘッダを読む。
// dev / 単体起動時のために query parameter `x-user-id` を fallback として認める。
// 本番では Envoy が JWT 検証後に必ずヘッダを上書きするので query は無視される運用。
func readUserID(r *http.Request) string {
	if v := r.Header.Get(auth.MetadataKeyUserID); v != "" {
		return v
	}
	return r.URL.Query().Get(auth.MetadataKeyUserID)
}

func (h *Handler) writeLoop(ctx context.Context, conn *websocket.Conn, c *hub.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		case payload, ok := <-c.Outbound():
			if !ok {
				return
			}
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Write(writeCtx, websocket.MessageText, payload)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func (h *Handler) readLoop(ctx context.Context, conn *websocket.Conn, userID, roomID string) {
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			// 通常の切断は debug ログで十分。
			if !errors.Is(err, context.Canceled) {
				h.logger.Debug("ws: read ended", "error", err)
			}
			return
		}
		var in Inbound
		if err := json.Unmarshal(data, &in); err != nil {
			h.sendError(ctx, conn, "invalid_json", err.Error())
			continue
		}
		switch in.Type {
		case TypeMessage:
			h.HandleMessage(ctx, userID, roomID, in.Content)
		default:
			h.sendError(ctx, conn, "unknown_type", "unknown message type: "+in.Type)
		}
	}
}

// HandleMessage はクライアント受信 → (a) 永続化 + (b) 配信 を並行実行する経路。
//
// 永続化と配信を 2 つの goroutine に分けるのは送信遅延を最小化するため:
// chat-service の DB 書き込み完了を待たずに Bob に届く (永続化は非同期で良い前提)。
// 失敗時は永続化エラーは log に流すだけ、配信は Redis に PUBLISH 完了で成功とみなす。
//
// 公開メソッドにしているのはテスト (handler_test) からの呼び出し用。
func (h *Handler) HandleMessage(ctx context.Context, userID, roomID, content string) {
	// chat-service が採番する ID と created_at が必要だが、永続化を待たずに配信したいので
	// 配信側ではこのプロセスで仮 ID と仮 created_at を採番する設計はせず、
	// 永続化レスポンスを使う方が正しい。ただし永続化レスポンスを待つと遅延が増える。
	// → トレードオフ: realtime path では永続化レスポンスを待たず、暫定 ID 無しで配信する。
	//   Phase 2 ではクライアントが受信メッセージ ID をすぐに必要としないと割り切る。
	now := h.now().UTC()
	out := Outbound{
		Type:      TypeMessage,
		RoomID:    roomID,
		SenderID:  userID,
		Content:   content,
		CreatedAt: now,
	}
	payload, err := json.Marshal(out)
	if err != nil {
		h.logger.Error("ws: marshal payload", "error", err)
		return
	}

	// (a) 永続化 (fire-and-forget)。エラーは log に残すのみ。
	go func() {
		// timeout は短め (chat-service が遅延しても WS 配信を巻き込まない)。
		persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.chatClient.SendMessage(persistCtx, userID, roomID, content); err != nil {
			h.logger.Error("ws: persist failed", "error", err, "user_id", userID, "room_id", roomID)
		}
	}()

	// (b) 配信 (Redis PUBLISH)。同じく非同期で。
	go func() {
		pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.pubsub.Publish(pubCtx, pubsub.RoomEvent{RoomID: roomID, Payload: payload}); err != nil {
			h.logger.Error("ws: publish failed", "error", err, "room_id", roomID)
		}
	}()
}

func (h *Handler) sendError(ctx context.Context, conn *websocket.Conn, code, msg string) {
	errOut := Outbound{Type: TypeError, Code: code, Message: msg}
	b, _ := json.Marshal(errOut)
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = conn.Write(writeCtx, websocket.MessageText, b)
}

