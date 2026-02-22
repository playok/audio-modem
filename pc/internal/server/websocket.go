package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for local development
	},
}

// WSMessage represents a WebSocket message.
type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// ProgressPayload represents a progress update.
type ProgressPayload struct {
	Status   string  `json:"status"`
	Message  string  `json:"message"`
	Progress float64 `json:"progress"` // 0.0 to 1.0
	BytesSent   int64  `json:"bytesSent,omitempty"`
	TotalBytes  int64  `json:"totalBytes,omitempty"`
}

// WSHub manages WebSocket connections.
type WSHub struct {
	clients map[*websocket.Conn]bool
	mu      sync.RWMutex
}

// NewWSHub creates a new WebSocket hub.
func NewWSHub() *WSHub {
	return &WSHub{
		clients: make(map[*websocket.Conn]bool),
	}
}

// AddClient registers a new WebSocket connection.
func (h *WSHub) AddClient(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = true
	log.Printf("WebSocket client connected (%d total)", len(h.clients))
}

// RemoveClient removes a WebSocket connection.
func (h *WSHub) RemoveClient(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, conn)
	conn.Close()
	log.Printf("WebSocket client disconnected (%d remaining)", len(h.clients))
}

// Broadcast sends a message to all connected clients.
func (h *WSHub) Broadcast(msg WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket marshal error: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn := range h.clients {
		err := conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			log.Printf("WebSocket write error: %v", err)
			go h.RemoveClient(conn)
		}
	}
}

// BroadcastProgress sends a progress update to all clients.
func (h *WSHub) BroadcastProgress(status, message string, progress float64, bytesSent, totalBytes int64) {
	h.Broadcast(WSMessage{
		Type: "progress",
		Payload: ProgressPayload{
			Status:     status,
			Message:    message,
			Progress:   progress,
			BytesSent:  bytesSent,
			TotalBytes: totalBytes,
		},
	})
}

// BroadcastStatus sends a status update to all clients.
func (h *WSHub) BroadcastStatus(status, message string) {
	h.Broadcast(WSMessage{
		Type: "status",
		Payload: map[string]string{
			"status":  status,
			"message": message,
		},
	})
}

// BroadcastLog sends a log message to all clients.
func (h *WSHub) BroadcastLog(level, message string) {
	h.Broadcast(WSMessage{
		Type: "log",
		Payload: map[string]string{
			"level":   level,
			"message": message,
		},
	})
}
