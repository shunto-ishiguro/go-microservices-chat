// Package pubsub はプロセス間メッセージ配送の抽象。
//
// 本番経路は Redis Pub/Sub (PUBLISH room:<id> + PSUBSCRIBE room:*)。
// テスト経路は同プロセス Go channel (Redis 不要で go test ./... が通る)。
//
// 「Pub/Sub を最初から通す」のが Phase 2 の核: 単一プロセス時でも本番と同じ経路を辿らせて
// おけば、infra 側でレプリカを 1 → N に増やしても app コードは変わらない。
package pubsub

import "context"

// RoomEvent は room チャネルに流れる 1 通分のペイロード。
// 配信内容の意味は ws レイヤー (ws/protocol.go) で定義する; pubsub は不透明バイト列として扱う。
type RoomEvent struct {
	RoomID  string
	Payload []byte
}

// Publisher は room 単位でイベントを発火する側 (= 受信した WS メッセージを fan-out する側)。
type Publisher interface {
	Publish(ctx context.Context, ev RoomEvent) error
}

// Subscriber は全 room を一括購読し、受信ごとに onMessage を呼ぶ。
//
// インスタンス生存期間中に 1 回だけ Subscribe を呼ぶ想定。Subscribe は ctx が Done になるか
// 致命的なエラーを返すまでブロックする (= 別 goroutine で起動する)。
type Subscriber interface {
	Subscribe(ctx context.Context, onMessage func(RoomEvent)) error
}

// PubSub は両 interface を満たす実装を 1 つの値で扱いたい場合の便宜的な合成。
// main では Publisher と Subscriber を別物として扱っても、同 1 値として扱っても良い。
type PubSub interface {
	Publisher
	Subscriber
}
