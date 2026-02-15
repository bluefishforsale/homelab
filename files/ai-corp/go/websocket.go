package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

// WSClient represents a connected WebSocket client
type WSClient struct {
	ID     uuid.UUID
	Conn   *websocket.Conn
	Send   chan []byte
	Hub    *WSHub
	mu     sync.Mutex
}

// WSHub manages all WebSocket connections
type WSHub struct {
	clients    map[*WSClient]bool
	broadcast  chan []byte
	register   chan *WSClient
	unregister chan *WSClient
	mu         sync.RWMutex
}

// NewWSHub creates a new WebSocket hub
func NewWSHub() *WSHub {
	return &WSHub{
		clients:    make(map[*WSClient]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
	}
}

// Run starts the hub's main loop
func (h *WSHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			wsConnections.Inc()
			log.WithField("client_id", client.ID).Debug("WebSocket client connected")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
			}
			h.mu.Unlock()
			wsConnections.Dec()
			log.WithField("client_id", client.ID).Debug("WebSocket client disconnected")

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a message to all connected clients
func (h *WSHub) Broadcast(msg WebSocketMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.WithError(err).Error("Failed to marshal WebSocket message")
		return
	}
	
	select {
	case h.broadcast <- data:
		wsMessages.WithLabelValues(msg.Type).Inc()
	default:
		log.Warn("WebSocket broadcast channel full")
	}
}

// ClientCount returns the number of connected clients
func (h *WSHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// writePump pumps messages from the hub to the websocket connection
func (c *WSClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.mu.Lock()
			err := c.Conn.WriteMessage(websocket.TextMessage, message)
			c.mu.Unlock()
			
			if err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump pumps messages from the websocket connection to the hub
func (c *WSClient) readPump() {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(512)
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.WithError(err).Debug("WebSocket read error")
			}
			break
		}
	}
}

// WSHandler handles WebSocket upgrade requests
func (h *Handlers) WSHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.WithError(err).Error("WebSocket upgrade failed")
		return
	}

	client := &WSClient{
		ID:   uuid.New(),
		Conn: conn,
		Send: make(chan []byte, 256),
		Hub:  h.app.wsHub,
	}

	h.app.wsHub.register <- client

	go client.writePump()
	go client.readPump()
}

// BroadcastOrgUpdate sends organization update to all clients
func (app *App) BroadcastOrgUpdate() {
	if app.wsHub == nil {
		return
	}
	
	stats := app.org.GetStats()
	app.wsHub.Broadcast(WebSocketMessage{
		Type:    "org_update",
		Payload: stats,
	})
}

// BroadcastEmployeeUpdate sends employee status update
func (app *App) BroadcastEmployeeUpdate(employeeID uuid.UUID, status string, workTitle string) {
	if app.wsHub == nil {
		return
	}
	
	app.wsHub.Broadcast(WebSocketMessage{
		Type: "employee_update",
		Payload: map[string]interface{}{
			"employee_id": employeeID,
			"status":      status,
			"work_title":  workTitle,
		},
	})
}

// BroadcastWorkComplete sends work completion notification
func (app *App) BroadcastWorkComplete(employeeID uuid.UUID, employeeName string, workTitle string, hasError bool) {
	if app.wsHub == nil {
		return
	}
	
	app.wsHub.Broadcast(WebSocketMessage{
		Type: "work_complete",
		Payload: map[string]interface{}{
			"employee_id":   employeeID,
			"employee_name": employeeName,
			"work_title":    workTitle,
			"has_error":     hasError,
		},
	})
}

// BroadcastSeedUpdate sends company seed update
func (app *App) BroadcastSeedUpdate(seed *CompanySeed) {
	if app.wsHub == nil {
		return
	}
	
	app.wsHub.Broadcast(WebSocketMessage{
		Type:    "seed_update",
		Payload: seed,
	})
}

// StartPeriodicBroadcast starts periodic org status broadcasts
func (app *App) StartPeriodicBroadcast() {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				if app.wsHub != nil && app.wsHub.ClientCount() > 0 {
					app.BroadcastOrgUpdate()
				}
			}
		}
	}()
}
