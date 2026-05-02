package ws_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"go-microservices-chat/pkg/auth"
	"go-microservices-chat/services/realtime-service/internal/chatclient"
	"go-microservices-chat/services/realtime-service/internal/hub"
	"go-microservices-chat/services/realtime-service/internal/pubsub"
	wsx "go-microservices-chat/services/realtime-service/internal/ws"
)

// quietLogger はテスト中に slog 出力を捨てるためのもの (失敗時の出力を見やすくする)。
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// startTestServer は WS handler を httptest.Server に乗せて、テスト経路一式を返す。
func startTestServer(t *testing.T) (string, *hub.Hub, *pubsub.InMem, *chatclient.Fake, func()) {
	t.Helper()
	h := hub.NewHub()
	go h.Run()

	ps := pubsub.NewInMem()
	cc := chatclient.NewFake()
	handler := wsx.NewHandler(quietLogger(), h, ps, cc)
	srv := httptest.NewServer(handler)

	cleanup := func() {
		srv.Close()
		ps.Close()
		h.Stop()
	}
	return srv.URL, h, ps, cc, cleanup
}

func TestHandler_RequiresUserIDAndRoomID(t *testing.T) {
	url, _, _, _, cleanup := startTestServer(t)
	defer cleanup()

	// user_id 無し
	resp, err := http.Get(url + "/ws?room_id=r1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("missing user_id: status = %d, want 401", resp.StatusCode)
	}

	// room_id 無し
	req, _ := http.NewRequest(http.MethodGet, url+"/ws", nil)
	req.Header.Set(auth.MetadataKeyUserID, "alice")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("missing room_id: status = %d, want 400", resp2.StatusCode)
	}
}

func TestHandler_ReceivedMessageIsPersistedAndPublished(t *testing.T) {
	url, _, ps, cc, cleanup := startTestServer(t)
	defer cleanup()

	wsURL := strings.Replace(url, "http://", "ws://", 1) + "/ws?room_id=r1"
	conn, _, err := websocket.Dial(context.Background(), wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{auth.MetadataKeyUserID: []string{"alice"}},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Subscribe を別 goroutine で起動して、Publish された payload を捕まえる。
	received := make(chan pubsub.RoomEvent, 1)
	subCtx, subCancel := context.WithCancel(context.Background())
	defer subCancel()
	go func() {
		_ = ps.Subscribe(subCtx, func(ev pubsub.RoomEvent) { received <- ev })
	}()

	in := wsx.Inbound{Type: wsx.TypeMessage, Content: "hello"}
	body, _ := json.Marshal(in)
	if err := conn.Write(context.Background(), websocket.MessageText, body); err != nil {
		t.Fatalf("write: %v", err)
	}

	// 永続化と Publish は別 goroutine で走るので少し待つ。
	select {
	case ev := <-received:
		if ev.RoomID != "r1" {
			t.Errorf("publish room = %q, want r1", ev.RoomID)
		}
		var out wsx.Outbound
		if err := json.Unmarshal(ev.Payload, &out); err != nil {
			t.Fatalf("payload not JSON: %v", err)
		}
		if out.Content != "hello" || out.SenderID != "alice" {
			t.Errorf("payload = %+v", out)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for publish")
	}

	// chat-service への永続化呼び出しも届いている (非同期なので少し待つ)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if calls := cc.Calls(); len(calls) == 1 {
			if calls[0].SenderID != "alice" || calls[0].RoomID != "r1" || calls[0].Content != "hello" {
				t.Errorf("persist call = %+v", calls[0])
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("persist call not observed: calls = %+v", cc.Calls())
}

func TestHandler_BroadcastFromHubReachesClient(t *testing.T) {
	// Subscriber → Hub → Client の経路を直接刺激する: Publish した payload が
	// 接続中のクライアントに WS で届くことを確認する。
	url, h, ps, _, cleanup := startTestServer(t)
	defer cleanup()

	// Subscriber を起動して Pub/Sub からの payload を hub に流す。本番経路の main の配線を再現。
	subCtx, subCancel := context.WithCancel(context.Background())
	defer subCancel()
	go func() {
		_ = ps.Subscribe(subCtx, func(ev pubsub.RoomEvent) {
			h.LocalBroadcast(hub.LocalEvent{RoomID: ev.RoomID, Payload: ev.Payload})
		})
	}()

	wsURL := strings.Replace(url, "http://", "ws://", 1) + "/ws?room_id=r1"
	conn, _, err := websocket.Dial(context.Background(), wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{auth.MetadataKeyUserID: []string{"bob"}},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// 接続後、Hub に登録されるまで少し待つ
	time.Sleep(50 * time.Millisecond)

	out := wsx.Outbound{Type: wsx.TypeMessage, RoomID: "r1", SenderID: "alice", Content: "hi"}
	payload, _ := json.Marshal(out)
	if err := ps.Publish(context.Background(), pubsub.RoomEvent{RoomID: "r1", Payload: payload}); err != nil {
		t.Fatal(err)
	}

	readCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, data, err := conn.Read(readCtx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got wsx.Outbound
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Content != "hi" || got.SenderID != "alice" {
		t.Errorf("got = %+v", got)
	}
}
