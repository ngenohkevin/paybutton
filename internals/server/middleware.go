package server

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ngenohkevin/paybutton/internals/monitoring"
	"github.com/ngenohkevin/paybutton/internals/payment_processing"
	"golang.org/x/time/rate"
)

// Rate limiter per IP
var (
	visitors = make(map[string]*rate.Limiter)
	mu       sync.RWMutex

	wsConnections = make(map[string]int)
	wsMu          sync.RWMutex

	sseConnections = make(map[string]int)
	sseMu          sync.RWMutex
)

// getVisitor returns rate limiter for IP
func getVisitor(ip string, requestsPerMinute int, burst int) *rate.Limiter {
	mu.RLock()
	limiter, exists := visitors[ip]
	mu.RUnlock()

	if !exists {
		limiter = rate.NewLimiter(rate.Every(time.Minute/time.Duration(requestsPerMinute)), burst)
		mu.Lock()
		visitors[ip] = limiter
		mu.Unlock()
	}

	return limiter
}

// cleanupVisitors removes old entries periodically
func cleanupVisitors() {
	for {
		time.Sleep(10 * time.Minute)
		mu.Lock()
		// Clear all visitors to prevent memory leak
		visitors = make(map[string]*rate.Limiter)
		mu.Unlock()
	}
}

func init() {
	go cleanupVisitors()
}

// rateLimitMiddleware implements per-IP rate limiting
func (s *Server) rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip rate limiting for admin endpoints and static files
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/admin") || strings.HasPrefix(path, "/static") {
			c.Next()
			return
		}

		ip := c.ClientIP()
		limiter := getVisitor(ip, s.config.RequestsPerMinute, s.config.BurstSize)

		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded. Please try again later.",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// websocketLimitMiddleware limits WebSocket connections per IP
func (s *Server) websocketLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Only apply to WebSocket endpoints - check path length first
		isWebSocket := len(path) >= 4 && path[:4] == "/ws/"
		isSSE := len(path) >= 8 && path[:8] == "/events/"

		if !isWebSocket && !isSSE {
			c.Next()
			return
		}

		ip := c.ClientIP()

		if isWebSocket {
			wsMu.RLock()
			count := wsConnections[ip]
			wsMu.RUnlock()

			if count >= s.config.MaxWebSockets/10 { // Limit per IP to 1/10 of total
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error": "Too many WebSocket connections from your IP",
				})
				c.Abort()
				return
			}
		}

		if isSSE {
			sseMu.RLock()
			count := sseConnections[ip]
			sseMu.RUnlock()

			if count >= s.config.MaxSSEConnections/10 { // Limit per IP to 1/10 of total
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error": "Too many SSE connections from your IP",
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// handleWebSocketWithLimit wraps WebSocket handler with connection limiting
func (s *Server) handleWebSocketWithLimit(c *gin.Context) {
	ip := c.ClientIP()

	// Check global limit
	wsMu.Lock()
	totalConnections := 0
	for _, count := range wsConnections {
		totalConnections += count
	}

	if totalConnections >= s.config.MaxWebSockets {
		wsMu.Unlock()
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Server WebSocket capacity reached. Please try again later.",
		})
		return
	}

	// Increment connection count
	wsConnections[ip]++
	wsMu.Unlock()

	// Ensure decrement on completion
	defer func() {
		wsMu.Lock()
		wsConnections[ip]--
		if wsConnections[ip] <= 0 {
			delete(wsConnections, ip)
		}
		wsMu.Unlock()
	}()

	// Check if we can start a new goroutine
	monitor := monitoring.GetResourceMonitor()
	if !monitor.CanStartGoroutine() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Server at capacity. Please try again later.",
		})
		return
	}

	// Call actual WebSocket handler
	payment_processing.HandleWebSocket(c)
}

// handleSSEWithLimit wraps SSE handler with connection limiting
func (s *Server) handleSSEWithLimit(c *gin.Context) {
	ip := c.ClientIP()

	// Check global limit
	sseMu.Lock()
	totalConnections := 0
	for _, count := range sseConnections {
		totalConnections += count
	}

	if totalConnections >= s.config.MaxSSEConnections {
		sseMu.Unlock()
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Server SSE capacity reached. Please try again later.",
		})
		return
	}

	// Increment connection count
	sseConnections[ip]++
	sseMu.Unlock()

	// Ensure decrement on completion
	defer func() {
		sseMu.Lock()
		sseConnections[ip]--
		if sseConnections[ip] <= 0 {
			delete(sseConnections, ip)
		}
		sseMu.Unlock()
	}()

	// Check if we can start a new goroutine
	monitor := monitoring.GetResourceMonitor()
	if !monitor.CanStartGoroutine() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Server at capacity. Please try again later.",
		})
		return
	}

	// Call actual SSE handler
	payment_processing.HandleSSE(c)
}
