package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/homelab-game/backend/internal/auth"
)

const (
	pingInterval = 30 * time.Second
	pongTimeout  = 45 * time.Second
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Message sent to clients over WebSocket.
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Hub manages all connected WebSocket clients.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]*websocket.Conn // userID -> conn
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]*websocket.Conn),
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

		h.mu.Lock()
		if old, ok := h.clients[claims.UserID]; ok {
			old.Close()
		}
		h.clients[claims.UserID] = conn
		h.mu.Unlock()

		log.Printf("WebSocket connected: %s", claims.UserID)

		// Set pong handler and initial read deadline
		conn.SetReadDeadline(time.Now().Add(pongTimeout))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(pongTimeout))
			return nil
		})

		// Ping ticker — keeps connection alive through nginx proxy
		go func() {
			ticker := time.NewTicker(pingInterval)
			defer ticker.Stop()
			for range ticker.C {
				h.mu.RLock()
				_, exists := h.clients[claims.UserID]
				h.mu.RUnlock()
				if !exists {
					return
				}
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			}
		}()

		// Read loop (keeps connection alive, handles client disconnect)
		go func() {
			defer func() {
				h.mu.Lock()
				delete(h.clients, claims.UserID)
				h.mu.Unlock()
				conn.Close()
				log.Printf("WebSocket disconnected: %s", claims.UserID)
			}()
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					break
				}
			}
		}()
	}
}

// SendToUser sends a message to a specific user if connected.
func (h *Hub) SendToUser(userID string, msg Message) {
	h.mu.RLock()
	conn, ok := h.clients[userID]
	h.mu.RUnlock()

	if !ok {
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		h.mu.Lock()
		delete(h.clients, userID)
		h.mu.Unlock()
		conn.Close()
	}
}

// ConnectedUsers returns the number of connected clients.
func (h *Hub) ConnectedUsers() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
