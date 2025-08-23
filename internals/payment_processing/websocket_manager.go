package payment_processing

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections from any origin (configure appropriately for production)
		origin := r.Header.Get("Origin")
		log.Printf("WebSocket connection attempt from origin: %s", origin)
		return true
	},
	EnableCompression: true,
}

// WebSocket connection management
type WebSocketManager struct {
	connections map[string][]*websocket.Conn // address -> list of connections
	mutex       sync.RWMutex
}

var wsManager = &WebSocketManager{
	connections: make(map[string][]*websocket.Conn),
}

// BalanceUpdate represents a balance update message
type BalanceUpdate struct {
	Address    string  `json:"address"`
	Status     string  `json:"status"`     // "waiting", "confirmed", "failed"
	Balance    float64 `json:"balance"`    // Balance in USD
	BalanceBTC float64 `json:"balanceBTC"` // Balance in BTC
	Timestamp  string  `json:"timestamp"`
	Email      string  `json:"email"`
}

// HandleWebSocket handles WebSocket connections for real-time balance updates
func HandleWebSocket(c *gin.Context) {
	address := c.Param("address")
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "address parameter required"})
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade WebSocket: %v", err)
		return
	}
	defer conn.Close()

	// Add connection to manager
	wsManager.addConnection(address, conn)
	defer wsManager.removeConnection(address, conn)

	// Update session tracking to show WebSocket connection
	if SessionWebSocketUpdater != nil {
		SessionWebSocketUpdater(address, true)
		defer SessionWebSocketUpdater(address, false)
	}

	log.Printf("WebSocket connected for address: %s", address)

	// Send initial waiting status
	initialUpdate := BalanceUpdate{
		Address:   address,
		Status:    "waiting",
		Balance:   0,
		Timestamp: getCurrentTimestamp(),
	}
	if err := conn.WriteJSON(initialUpdate); err != nil {
		log.Printf("Error sending initial WebSocket message: %v", err)
		return
	}

	// Keep connection alive and handle disconnection
	for {
		// Read message from client (ping/pong or close)
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket unexpected close error: %v", err)
			}
			break
		}
	}
}

// addConnection adds a WebSocket connection for an address
func (wm *WebSocketManager) addConnection(address string, conn *websocket.Conn) {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()

	if wm.connections[address] == nil {
		wm.connections[address] = make([]*websocket.Conn, 0)
	}
	wm.connections[address] = append(wm.connections[address], conn)

	log.Printf("Added WebSocket connection for address %s (total: %d)", address, len(wm.connections[address]))
}

// removeConnection removes a WebSocket connection for an address
func (wm *WebSocketManager) removeConnection(address string, conn *websocket.Conn) {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()

	connections := wm.connections[address]
	for i, c := range connections {
		if c == conn {
			// Remove connection from slice
			wm.connections[address] = append(connections[:i], connections[i+1:]...)
			break
		}
	}

	// Clean up empty address entries
	if len(wm.connections[address]) == 0 {
		delete(wm.connections, address)
	}

	log.Printf("Removed WebSocket connection for address %s (remaining: %d)", address, len(wm.connections[address]))
}

// BroadcastBalanceUpdate sends balance update to all connections for an address
func BroadcastBalanceUpdate(address string, status string, balance float64, balanceBTC float64, email string) {
	wm := wsManager
	wm.mutex.RLock()
	connections := wm.connections[address]
	wm.mutex.RUnlock()

	if len(connections) == 0 {
		return // No WebSocket connections for this address
	}

	update := BalanceUpdate{
		Address:    address,
		Status:     status,
		Balance:    balance,
		BalanceBTC: balanceBTC,
		Timestamp:  getCurrentTimestamp(),
		Email:      email,
	}

	// Send to all connections for this address
	for _, conn := range connections {
		if err := conn.WriteJSON(update); err != nil {
			log.Printf("Error sending WebSocket message to %s: %v", address, err)
			// Remove failed connection
			wm.removeConnection(address, conn)
		}
	}

	log.Printf("Broadcasted balance update for %s: %s (%.8f BTC / $%.2f)", address, status, balanceBTC, balance)
}

// getCurrentTimestamp returns the current timestamp in RFC3339 format
func getCurrentTimestamp() string {
	return getCurrentTime()
}

// getCurrentTime returns current time
func getCurrentTime() string {
	return time.Now().Format(time.RFC3339)
}

// SSE (Server-Sent Events) Management
type SSEManager struct {
	connections map[string][]chan string // address -> list of SSE channels
	mutex       sync.RWMutex
}

var sseManager = &SSEManager{
	connections: make(map[string][]chan string),
}

// HandleSSE handles Server-Sent Events connections for real-time balance updates
func HandleSSE(c *gin.Context) {
	address := c.Param("address")
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "address parameter required"})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	// Create channel for this connection
	eventChan := make(chan string, 10)

	// Add to SSE manager
	sseManager.addSSEConnection(address, eventChan)
	defer sseManager.removeSSEConnection(address, eventChan)

	log.Printf("SSE connected for address: %s", address)

	// Send initial waiting event
	initialEvent := fmt.Sprintf("data: {\"address\":\"%s\",\"status\":\"waiting\",\"balance\":0,\"timestamp\":\"%s\"}\n\n",
		address, getCurrentTimestamp())

	c.Writer.WriteString(initialEvent)
	c.Writer.Flush()

	// Listen for events or client disconnect
	clientGone := c.Writer.CloseNotify()
	for {
		select {
		case event := <-eventChan:
			// Send event to client
			c.Writer.WriteString(event)
			c.Writer.Flush()
		case <-clientGone:
			log.Printf("SSE client disconnected for address: %s", address)
			return
		}
	}
}

// addSSEConnection adds an SSE connection for an address
func (sm *SSEManager) addSSEConnection(address string, eventChan chan string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.connections[address] == nil {
		sm.connections[address] = make([]chan string, 0)
	}
	sm.connections[address] = append(sm.connections[address], eventChan)

	log.Printf("Added SSE connection for address %s (total: %d)", address, len(sm.connections[address]))
}

// removeSSEConnection removes an SSE connection for an address
func (sm *SSEManager) removeSSEConnection(address string, eventChan chan string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	channels := sm.connections[address]
	for i, ch := range channels {
		if ch == eventChan {
			// Close channel and remove from slice
			close(ch)
			sm.connections[address] = append(channels[:i], channels[i+1:]...)
			break
		}
	}

	// Clean up empty address entries
	if len(sm.connections[address]) == 0 {
		delete(sm.connections, address)
	}

	log.Printf("Removed SSE connection for address %s (remaining: %d)", address, len(sm.connections[address]))
}

// BroadcastSSEUpdate sends balance update to all SSE connections for an address
func BroadcastSSEUpdate(address string, status string, balance float64, balanceBTC float64, email string) {
	sm := sseManager
	sm.mutex.RLock()
	channels := sm.connections[address]
	sm.mutex.RUnlock()

	if len(channels) == 0 {
		return // No SSE connections for this address
	}

	// Format SSE event
	event := fmt.Sprintf("data: {\"address\":\"%s\",\"status\":\"%s\",\"balance\":%.2f,\"balanceBTC\":%.8f,\"timestamp\":\"%s\",\"email\":\"%s\"}\n\n",
		address, status, balance, balanceBTC, getCurrentTimestamp(), email)

	// Send to all SSE connections for this address
	for _, ch := range channels {
		select {
		case ch <- event:
			// Event sent successfully
		default:
			// Channel is full or closed, skip
			log.Printf("SSE channel full/closed for address %s", address)
		}
	}

	log.Printf("Broadcasted SSE update for %s: %s (%.8f BTC / $%.2f)", address, status, balanceBTC, balance)
}

// Enhanced BroadcastBalanceUpdate that sends to both WebSocket AND SSE
func BroadcastBalanceUpdateAll(address string, status string, balance float64, balanceBTC float64, email string) {
	// Send to WebSocket connections
	BroadcastBalanceUpdate(address, status, balance, balanceBTC, email)

	// Send to SSE connections
	BroadcastSSEUpdate(address, status, balance, balanceBTC, email)
}
