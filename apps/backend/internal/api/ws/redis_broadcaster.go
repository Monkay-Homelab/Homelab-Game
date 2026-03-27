package ws

import (
	"context"
	"encoding/json"
	"log"

	"github.com/redis/go-redis/v9"
)

const wsBroadcastChannel = "ws:broadcast"

type broadcastMessage struct {
	UserID string `json:"u"`
	Data   []byte `json:"d"`
}

// RedisBroadcaster implements MessageBroadcaster using Redis pub/sub.
type RedisBroadcaster struct {
	hub    *Hub
	rdb    *redis.Client
	pubsub *redis.PubSub
	done   chan struct{}
}

func NewRedisBroadcaster(hub *Hub, rdb *redis.Client) *RedisBroadcaster {
	return &RedisBroadcaster{
		hub:  hub,
		rdb:  rdb,
		done: make(chan struct{}),
	}
}

func (b *RedisBroadcaster) SendToUser(userID string, msg Message) {
	// Fast path: deliver locally if connected
	if b.hub.HasUser(userID) {
		b.hub.SendToUser(userID, msg)
		return
	}
	// Slow path: publish to Redis
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	bm := broadcastMessage{UserID: userID, Data: data}
	payload, _ := json.Marshal(bm)
	if err := b.rdb.Publish(context.Background(), wsBroadcastChannel, payload).Err(); err != nil {
		log.Printf("[ws-pubsub] Redis publish error: %v", err)
	}
}

func (b *RedisBroadcaster) SendToUserBytes(userID string, data []byte) {
	if b.hub.HasUser(userID) {
		b.hub.SendToUserBytes(userID, data)
		return
	}
	bm := broadcastMessage{UserID: userID, Data: data}
	payload, _ := json.Marshal(bm)
	if err := b.rdb.Publish(context.Background(), wsBroadcastChannel, payload).Err(); err != nil {
		log.Printf("[ws-pubsub] Redis publish error: %v", err)
	}
}

func (b *RedisBroadcaster) Start(ctx context.Context) error {
	b.pubsub = b.rdb.Subscribe(ctx, wsBroadcastChannel)
	go b.listen()
	return nil
}

func (b *RedisBroadcaster) listen() {
	ch := b.pubsub.Channel()
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var bm broadcastMessage
			if json.Unmarshal([]byte(msg.Payload), &bm) == nil {
				b.hub.SendToUserBytes(bm.UserID, bm.Data)
			}
		case <-b.done:
			return
		}
	}
}

func (b *RedisBroadcaster) Stop() {
	close(b.done)
	if b.pubsub != nil {
		b.pubsub.Close()
	}
}
