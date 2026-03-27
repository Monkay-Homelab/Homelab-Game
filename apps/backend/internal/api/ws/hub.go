package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/homelab-game/backend/internal/auth"
)

const (
	pingInterval = 30 * time.Second
	pongTimeout  = 45 * time.Second

	// sendBufSize is the capacity of the per-client outbound message channel.
	// At a 5-second push interval, 16 slots means a client must be unresponsive
	// for ~80 seconds before drops begin. The pong timeout (45s) will close the
	// connection well before that.
	sendBufSize = 16

	// writeWait is the deadline for writing a single message to the connection.
	writeWait = 10 * time.Second
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		allowed := map[string]bool{
			"https://game.homelab.living": true,
			"http://game.homelab.living":  true,
			"https://homelab.living":      true,
			"http://homelab.living":       true,
		}
		// Dev mode: allow localhost (mirror CORS middleware behavior)
		if os.Getenv("ENV") != "production" {
			allowed["http://localhost:3000"] = true
			allowed["http://127.0.0.1:3000"] = true
		}
		// Allow extra origins from env (comma-separated)
		if extra := os.Getenv("CORS_ORIGINS"); extra != "" {
			for _, o := range strings.Split(extra, ",") {
				allowed[strings.TrimSpace(o)] = true
			}
		}
		return allowed[origin]
	},
}

// Message sent to clients over WebSocket.
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Client wraps a single WebSocket connection with a buffered send channel.
// All writes to the underlying conn go through the writePump goroutine,
// eliminating concurrent write hazards.
type Client struct {
	UserID string
	conn   *websocket.Conn
	send   chan []byte    // buffered outbound message channel
	hub    *Hub
	done   chan struct{} // signals shutdown to tick goroutine (Phase 1)
}

// Hub manages all connected WebSocket clients.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]*Client // userID -> client

	// OnConnect is called after a client is registered and pumps are started.
	// The done channel is closed when the client disconnects, allowing the
	// callback owner to stop any per-user goroutines (e.g., tick timer).
	OnConnect func(userID string, done <-chan struct{})

	// OnDisconnect is called during cleanup after the done channel has been
	// closed. The tick goroutine has already been signaled to stop.
	OnDisconnect func(userID string)

	// OnMessage is called when a client sends a message over the WebSocket.
	// The hub routes the raw message bytes to this callback for processing
	// (e.g., game action handling). If nil, incoming messages are discarded.
	OnMessage func(userID string, data []byte)
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]*Client),
	}
}

// HandleConnect upgrades HTTP to WebSocket and registers the client.
func (h *Hub) HandleConnect(jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}

		claims, err := auth.ValidateToken(token, jwtSecret)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}

		client := &Client{
			UserID: claims.UserID,
			conn:   conn,
			send:   make(chan []byte, sendBufSize),
			hub:    h,
			done:   make(chan struct{}),
		}

		// Close any existing connection for this user (single connection per user).
		h.mu.Lock()
		if old, ok := h.clients[claims.UserID]; ok {
			close(old.send)
			// done is closed by writePump exit or readPump cleanup; closing send
			// causes writePump to exit, which triggers the close frame write.
			//
			// Fire OnDisconnect for the old connection so the old tick goroutine
			// stops before the new one starts. We close old.done here explicitly
			// to signal the tick goroutine immediately (readPump cleanup will
			// no-op on the already-closed channel via recover).
			select {
			case <-old.done:
				// already closed
			default:
				close(old.done)
			}
			if h.OnDisconnect != nil {
				h.OnDisconnect(old.UserID)
			}
		}
		h.clients[claims.UserID] = client
		h.mu.Unlock()

		log.Println("WebSocket client connected")

		go client.writePump()
		go client.readPump()

		// Notify after registration and pump start so the callback can
		// immediately send messages to the client.
		if h.OnConnect != nil {
			h.OnConnect(claims.UserID, client.done)
		}
	}
}

// writePump is the sole goroutine that writes to the WebSocket connection.
// It reads outbound messages from the send channel and owns the ping ticker.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				// send channel closed; write a close frame and exit.
				c.conn.SetWriteDeadline(time.Now().Add(writeWait))
				c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump reads from the WebSocket connection, routing incoming messages to
// the hub's OnMessage callback. It detects client disconnection and is
// responsible for cleanup.
func (c *Client) readPump() {
	defer func() {
		// Close done channel to signal tick goroutine shutdown (Phase 1).
		// Guard against double-close: if this client was replaced by a new
		// connection, HandleConnect already closed done.
		select {
		case <-c.done:
			// already closed
		default:
			close(c.done)
		}

		// Remove this client from the hub. Only remove if we are still the
		// registered client for this user (a new connection may have already
		// replaced us).
		c.hub.mu.Lock()
		if c.hub.clients[c.UserID] == c {
			delete(c.hub.clients, c.UserID)
			// Close the send channel, which causes writePump to exit and
			// write a close frame.
			close(c.send)

			// Notify after done is closed so the tick goroutine has already
			// been signaled to stop.
			if c.hub.OnDisconnect != nil {
				c.hub.OnDisconnect(c.UserID)
			}
		}
		c.hub.mu.Unlock()

		log.Println("WebSocket client disconnected")
	}()

	c.conn.SetReadDeadline(time.Now().Add(pongTimeout))
	c.conn.SetReadLimit(65536) // 64KB max incoming message size
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongTimeout))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		if c.hub.OnMessage != nil {
			c.hub.OnMessage(c.UserID, message)
		}
	}
}

// SendToUser sends a message to a specific user if connected.
// The message is written to the client's send channel (non-blocking).
// If the channel is full (slow client), the message is dropped and a
// warning is logged. The client will recover on the next push.
func (h *Hub) SendToUser(userID string, msg Message) {
	h.mu.RLock()
	client, ok := h.clients[userID]
	h.mu.RUnlock()

	if !ok {
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	select {
	case client.send <- data:
		// Message queued.
	default:
		log.Printf("WS push dropped for user %s: send buffer full", userID)
	}
}

// SendToUserBytes sends pre-serialized bytes to a specific user if connected.
// This avoids double-serialization when the caller has already marshalled the
// message (e.g., the GameHandler pushing a pre-built state response).
// If the channel is full (slow client), the message is dropped.
func (h *Hub) SendToUserBytes(userID string, data []byte) {
	h.mu.RLock()
	client, ok := h.clients[userID]
	h.mu.RUnlock()

	if !ok {
		return
	}

	select {
	case client.send <- data:
		// Message queued.
	default:
		log.Printf("WS push dropped for user %s: send buffer full", userID)
	}
}

// HasUser returns true if the given user has an active WebSocket connection on this replica.
func (h *Hub) HasUser(userID string) bool {
	h.mu.RLock()
	_, ok := h.clients[userID]
	h.mu.RUnlock()
	return ok
}

// ConnectedUsers returns the number of connected clients.
func (h *Hub) ConnectedUsers() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
