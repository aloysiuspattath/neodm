package server

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"ifd/backend/engine"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow UI and browser extensions
	},
}

// Hub manages active WebSocket client connections and broadcasts.
type Hub struct {
	mu         sync.Mutex
	clients    map[*websocket.Conn]bool
	broadcast  chan []byte
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	engine     *engine.Engine
}

// WSMessage represents a real-time event sent to the frontend.
type WSMessage struct {
	Type        string                 `json:"type"` // "telemetry", "task_update", "clipboard_url", "completed"
	Payload     interface{}            `json:"payload"`
	Timestamp   int64                  `json:"timestamp"`
	GlobalSpeed int64                  `json:"globalSpeed"`
}

// NewHub creates a new WebSocket hub.
func NewHub(eng *engine.Engine) *Hub {
	return &Hub{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
		engine:     eng,
	}
}

// Run begins the hub event loop.
func (h *Hub) Run() {
	// Telemetry broadcast ticker (every 150ms for buttery-smooth 60fps UI updates)
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.Lock()
			for client := range h.clients {
				err := client.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					client.Close()
					delete(h.clients, client)
				}
			}
			h.mu.Unlock()

		case <-ticker.C:
			h.broadcastTelemetry()
		}
	}
}

func (h *Hub) broadcastTelemetry() {
	tasks := h.engine.GetAllTasks()
	var globalSpeed int64 = 0

	for _, t := range tasks {
		if t.Status == "Downloading" {
			globalSpeed += t.Speed
		}
	}

	msg := WSMessage{
		Type:        "telemetry",
		Payload:     tasks,
		Timestamp:   time.Now().UnixMilli(),
		GlobalSpeed: globalSpeed,
	}

	data, err := json.Marshal(msg)
	if err == nil {
		h.Broadcast(data)
	}
}

// Broadcast sends a message to all connected clients.
func (h *Hub) Broadcast(data []byte) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.clients {
		_ = client.WriteMessage(websocket.TextMessage, data)
	}
}

// BroadcastEvent sends a custom typed event.
func (h *Hub) BroadcastEvent(eventType string, payload interface{}) {
	if h == nil {
		return
	}
	msg := WSMessage{
		Type:      eventType,
		Payload:   payload,
		Timestamp: time.Now().UnixMilli(),
	}
	if data, err := json.Marshal(msg); err == nil {
		h.Broadcast(data)
	}
}

// HandleWS upgrades HTTP connection to WebSocket.
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.register <- conn

	// Keep alive / read pump
	go func() {
		defer func() {
			h.unregister <- conn
		}()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}
