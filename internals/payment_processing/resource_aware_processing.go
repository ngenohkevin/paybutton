package payment_processing

import (
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/gorilla/websocket"
	"github.com/ngenohkevin/paybutton/internals/monitoring"
)

// StartBalanceCheckWithResourceLimit starts balance checking with resource limits
func StartBalanceCheckWithResourceLimit(address, email, token string, bot *tgbotapi.BotAPI, interval time.Duration) bool {
	monitor := monitoring.GetResourceMonitor()

	// Try to start goroutine with resource limits
	success := monitor.RunWithLimit(func() {
		checkBalanceWithInterval(address, email, token, bot, interval)
	})

	if !success {
		log.Printf("Warning: Cannot start balance check for %s - resource limit reached", address)
		// Could queue this for later or use a fallback mechanism
		return false
	}

	return true
}

// CleanupStaleConnections removes inactive WebSocket/SSE connections and old sessions
func CleanupStaleConnections() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		// Clean up old user sessions
		mutex.Lock()
		now := time.Now()
		for email, session := range userSessions {
			// Remove sessions older than 35 minutes (payment timeout + buffer)
			if now.Sub(session.LastActivity) > 35*time.Minute {
				delete(userSessions, email)
				log.Printf("Cleaned up stale session for %s", email)
			}
		}

		// Clean up checking addresses that have been monitoring for too long
		for addr, startTime := range checkingAddressesTime {
			if now.Sub(startTime) > 35*time.Minute {
				delete(checkingAddresses, addr)
				delete(checkingAddressesTime, addr)
				log.Printf("Stopped monitoring stale address %s", addr)
			}
		}
		mutex.Unlock()

		// Clean up WebSocket connections
		wsManager.mutex.Lock()
		for addr, conns := range wsManager.connections {
			// Remove nil connections
			validConns := make([]*websocket.Conn, 0)
			for _, conn := range conns {
				if conn != nil {
					// Try to ping the connection
					if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second)); err == nil {
						validConns = append(validConns, conn)
					} else {
						conn.Close()
						log.Printf("Closed stale WebSocket connection for %s", addr)
					}
				}
			}

			if len(validConns) > 0 {
				wsManager.connections[addr] = validConns
			} else {
				delete(wsManager.connections, addr)
			}
		}
		wsManager.mutex.Unlock()

		// Clean up SSE connections
		sseManager.mutex.Lock()
		for addr, channels := range sseManager.connections {
			// Check if channels are still active
			activeChannels := make([]chan string, 0)
			for _, ch := range channels {
				select {
				case ch <- "ping":
					activeChannels = append(activeChannels, ch)
				default:
					// Channel is blocked or closed
					close(ch)
					log.Printf("Closed stale SSE connection for %s", addr)
				}
			}

			if len(activeChannels) > 0 {
				sseManager.connections[addr] = activeChannels
			} else {
				delete(sseManager.connections, addr)
			}
		}
		sseManager.mutex.Unlock()

		log.Printf("Cleanup cycle completed - Sessions: %d, Monitoring: %d, WebSockets: %d, SSE: %d",
			len(userSessions), len(checkingAddresses), len(wsManager.connections), len(sseManager.connections))
	}
}

// Initialize cleanup routine
func init() {
	go CleanupStaleConnections()
}
