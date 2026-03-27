package ws

import "context"

// MessageBroadcaster abstracts the mechanism for sending messages to users.
// In single-replica mode, this delegates directly to the Hub.
// In multi-replica mode, this publishes to Redis pub/sub.
type MessageBroadcaster interface {
	// SendToUser sends a message to the specified user, regardless of which
	// replica they are connected to.
	SendToUser(userID string, msg Message)

	// SendToUserBytes sends pre-serialized bytes to the specified user.
	SendToUserBytes(userID string, data []byte)

	// Start begins listening for messages from other replicas.
	Start(ctx context.Context) error

	// Stop shuts down the broadcaster.
	Stop()
}

// LocalBroadcaster wraps a Hub for single-replica mode (no Redis).
type LocalBroadcaster struct {
	hub *Hub
}

func NewLocalBroadcaster(hub *Hub) *LocalBroadcaster {
	return &LocalBroadcaster{hub: hub}
}

func (b *LocalBroadcaster) SendToUser(userID string, msg Message) {
	b.hub.SendToUser(userID, msg)
}

func (b *LocalBroadcaster) SendToUserBytes(userID string, data []byte) {
	b.hub.SendToUserBytes(userID, data)
}

func (b *LocalBroadcaster) Start(_ context.Context) error { return nil }
func (b *LocalBroadcaster) Stop()                         {}
