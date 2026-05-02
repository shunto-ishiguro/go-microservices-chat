package pubsub

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

// channelPrefix は Redis 上で使う channel 名前空間。"room:<room_id>" の形で PUBLISH する。
// Subscriber 側は PSUBSCRIBE "room:*" で全 room を一括購読する設計。
const channelPrefix = "room:"

// Redis は go-redis を使った本番用 PubSub 実装。
//
// realtime-service は普通 1 プロセスに 1 個この値を持ち、Publish と Subscribe を兼ねる。
// 起動時の Subscribe goroutine が hub.LocalBroadcast に流す配線は main で組む。
type Redis struct {
	rdb *redis.Client
}

func NewRedis(addr string) *Redis {
	return &Redis{rdb: redis.NewClient(&redis.Options{Addr: addr})}
}

// Ping は起動時の疎通確認用。Redis が落ちていたら main で起動を断念する。
func (p *Redis) Ping(ctx context.Context) error {
	return p.rdb.Ping(ctx).Err()
}

func (p *Redis) Close() error { return p.rdb.Close() }

func (p *Redis) Publish(ctx context.Context, ev RoomEvent) error {
	return p.rdb.Publish(ctx, channelPrefix+ev.RoomID, ev.Payload).Err()
}

// Subscribe は PSUBSCRIBE "room:*" で全 room を一括購読し、受信ごとに onMessage を呼ぶ。
// ctx Done か接続エラーで return する。main 側は別 goroutine で起動して err を log するだけ。
func (p *Redis) Subscribe(ctx context.Context, onMessage func(RoomEvent)) error {
	ps := p.rdb.PSubscribe(ctx, channelPrefix+"*")
	defer ps.Close()
	ch := ps.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return fmt.Errorf("pubsub.redis: subscription channel closed")
			}
			roomID := strings.TrimPrefix(msg.Channel, channelPrefix)
			onMessage(RoomEvent{RoomID: roomID, Payload: []byte(msg.Payload)})
		}
	}
}
