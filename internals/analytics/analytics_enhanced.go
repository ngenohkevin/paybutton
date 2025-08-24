package analytics

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Enhanced analytics structures for v2

// BatchEvent represents a batch of analytics events
type BatchEvent struct {
	Type      string           `json:"type"`
	Events    []AnalyticsEvent `json:"events"`
	SessionID string           `json:"sessionId"`
	Timestamp string           `json:"timestamp"`
}

// AnalyticsEvent represents a single analytics event
type AnalyticsEvent struct {
	Type      string                 `json:"type"`
	Page      *PageInfo              `json:"page,omitempty"`
	Metrics   map[string]interface{} `json:"metrics,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Data      interface{}            `json:"data,omitempty"`
	Timestamp string                 `json:"timestamp"`
	SessionID string                 `json:"sessionId"`
}

// PageInfo contains page-specific information
type PageInfo struct {
	Path      string `json:"path"`
	Title     string `json:"title"`
	Referrer  string `json:"referrer"`
	URL       string `json:"url"`
	Timestamp string `json:"timestamp"`
}

// EngagementMetrics tracks user engagement
type EngagementMetrics struct {
	TimeOnPage   int `json:"timeOnPage"`   // seconds
	ScrollDepth  int `json:"scrollDepth"`  // percentage
	Interactions int `json:"interactions"` // click count
}

// PerformanceMetrics tracks page performance
type PerformanceMetrics struct {
	LoadTime             float64 `json:"loadTime"`
	DOMContentLoaded     float64 `json:"domContentLoaded"`
	FirstPaint           float64 `json:"firstPaint"`
	FirstContentfulPaint float64 `json:"firstContentfulPaint"`
}

// SessionMetrics tracks session-level metrics
type SessionMetrics struct {
	Duration     int64               `json:"duration"` // milliseconds
	PageViews    int                 `json:"pageViews"`
	Events       int                 `json:"events"`
	Engagement   *EngagementMetrics  `json:"engagement,omitempty"`
	Performance  *PerformanceMetrics `json:"performance,omitempty"`
	LastActivity time.Time           `json:"lastActivity"`
}

// EnhancedAnalyticsConnection extends the basic connection with more tracking
type EnhancedAnalyticsConnection struct {
	*AnalyticsConnection
	metrics       *SessionMetrics
	lastHeartbeat time.Time
	version       string
}

// BeaconPayload represents data sent via beacon API fallback
type BeaconPayload struct {
	Site      string           `json:"site"`
	SessionID string           `json:"sessionId"`
	Events    []AnalyticsEvent `json:"events"`
}

// Global enhanced features
var (
	sessionMetrics      map[string]*SessionMetrics
	sessionMetricsMutex sync.RWMutex
	beaconBuffer        []BeaconPayload
	beaconBufferMutex   sync.Mutex
)

func init() {
	sessionMetrics = make(map[string]*SessionMetrics)
	beaconBuffer = make([]BeaconPayload, 0, 1000)
}

// HandleEnhancedWebSocket handles v2 WebSocket connections with improved features
func HandleEnhancedWebSocket(c *gin.Context) {
	siteName := c.Param("siteName")
	if siteName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "siteName parameter required"})
		return
	}

	// Get version from query params
	version := c.DefaultQuery("v", "1.0")

	// Get additional context
	pagePath := c.DefaultQuery("path", "/")
	timezone := c.DefaultQuery("tz", "0")

	// Detect client capabilities
	userAgent := c.GetHeader("User-Agent")
	acceptEncoding := c.GetHeader("Accept-Encoding")
	supportsCompression := acceptEncoding != "" && (acceptEncoding == "*" || acceptEncoding == "gzip" || acceptEncoding == "deflate")

	logger.Info("Enhanced WebSocket connection attempt",
		slog.String("site", siteName),
		slog.String("version", version),
		slog.String("path", pagePath),
		slog.String("timezone", timezone),
		slog.Bool("compression", supportsCompression))

	// Upgrade with compression if supported
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("Failed to upgrade enhanced WebSocket",
			slog.String("site", siteName),
			slog.String("error", err.Error()))
		return
	}
	defer conn.Close()

	// Generate session ID
	sessionID := generateSessionID()

	// Create enhanced connection
	enhancedConn := &EnhancedAnalyticsConnection{
		AnalyticsConnection: &AnalyticsConnection{
			conn:      conn,
			sessionID: sessionID,
			siteName:  siteName,
			joinTime:  time.Now(),
			pagePath:  pagePath,
			region:    detectTorFriendlyRegion(userAgent, c.GetHeader("Accept-Language"), timezone),
		},
		metrics: &SessionMetrics{
			Duration:     0,
			PageViews:    0,
			Events:       0,
			LastActivity: time.Now(),
		},
		lastHeartbeat: time.Now(),
		version:       version,
	}

	// Store session metrics
	sessionMetricsMutex.Lock()
	sessionMetrics[sessionID] = enhancedConn.metrics
	sessionMetricsMutex.Unlock()

	// Add to manager
	if manager != nil {
		manager.addConnectionWithPath(siteName, conn, sessionID, pagePath, enhancedConn.region)
		defer manager.removeConnection(siteName, sessionID)
	}

	// Send initial configuration
	config := map[string]interface{}{
		"type":              "config",
		"status":            "connected",
		"sessionId":         sessionID,
		"heartbeatInterval": 30000, // 30 seconds
		"batchInterval":     5000,  // 5 seconds
		"timestamp":         time.Now().Format(time.RFC3339),
	}

	if err := conn.WriteJSON(config); err != nil {
		logger.Error("Error sending config",
			slog.String("site", siteName),
			slog.String("error", err.Error()))
		return
	}

	// Enhanced connection handling
	conn.SetReadDeadline(time.Now().Add(45 * time.Second)) // Longer timeout for v2
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(45 * time.Second))
		enhancedConn.lastHeartbeat = time.Now()
		return nil
	})

	// Message handling loop
	for {
		var message json.RawMessage
		err := conn.ReadJSON(&message)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
				logger.Warn("Enhanced WebSocket unexpected close",
					slog.String("site", siteName),
					slog.String("session", sessionID),
					slog.String("error", err.Error()))
			}
			break
		}

		// Process message based on type
		var msgType struct {
			Type string `json:"type"`
		}

		if err := json.Unmarshal(message, &msgType); err != nil {
			continue
		}

		switch msgType.Type {
		case "batch":
			handleBatchEvents(enhancedConn, message)

		case "heartbeat":
			handleHeartbeat(enhancedConn, message)

		case "pageview":
			handlePageView(enhancedConn, message)

		case "engagement":
			handleEngagement(enhancedConn, message)

		case "performance":
			handlePerformance(enhancedConn, message)

		case "custom":
			handleCustomEvent(enhancedConn, message)

		case "disconnect":
			handleDisconnect(enhancedConn, message)
			return

		default:
			logger.Debug("Unknown message type",
				slog.String("type", msgType.Type),
				slog.String("session", sessionID))
		}

		// Update last activity
		enhancedConn.metrics.LastActivity = time.Now()

		// Reset read deadline
		conn.SetReadDeadline(time.Now().Add(45 * time.Second))
	}

	// Calculate session duration
	enhancedConn.metrics.Duration = time.Since(enhancedConn.joinTime).Milliseconds()

	// Log session summary
	logger.Info("Enhanced session ended",
		slog.String("site", siteName),
		slog.String("session", sessionID),
		slog.Int64("duration_ms", enhancedConn.metrics.Duration),
		slog.Int("page_views", enhancedConn.metrics.PageViews),
		slog.Int("events", enhancedConn.metrics.Events))
}

// Handle batch events
func handleBatchEvents(conn *EnhancedAnalyticsConnection, message json.RawMessage) {
	var batch BatchEvent
	if err := json.Unmarshal(message, &batch); err != nil {
		logger.Error("Failed to parse batch event", slog.String("error", err.Error()))
		return
	}

	// Process each event
	for _, event := range batch.Events {
		conn.metrics.Events++

		switch event.Type {
		case "pageview":
			conn.metrics.PageViews++
		case "engagement":
			if metrics, ok := event.Metrics["engagement"].(*EngagementMetrics); ok {
				conn.metrics.Engagement = metrics
			}
		case "performance":
			if metrics, ok := event.Metrics["performance"].(*PerformanceMetrics); ok {
				conn.metrics.Performance = metrics
			}
		}
	}

	// Send acknowledgment
	ack := map[string]interface{}{
		"type":      "ack",
		"count":     len(batch.Events),
		"timestamp": time.Now().Format(time.RFC3339),
	}

	if err := conn.conn.WriteJSON(ack); err != nil {
		logger.Error("Failed to send ack", slog.String("error", err.Error()))
	}

	logger.Debug("Processed batch events",
		slog.String("session", conn.sessionID),
		slog.Int("count", len(batch.Events)))
}

// Handle heartbeat
func handleHeartbeat(conn *EnhancedAnalyticsConnection, message json.RawMessage) {
	conn.lastHeartbeat = time.Now()

	var heartbeat struct {
		Metrics struct {
			QueueSize int   `json:"queueSize"`
			Uptime    int64 `json:"uptime"`
		} `json:"metrics"`
	}

	if err := json.Unmarshal(message, &heartbeat); err == nil {
		logger.Debug("Heartbeat received",
			slog.String("session", conn.sessionID),
			slog.Int("queue_size", heartbeat.Metrics.QueueSize),
			slog.Int64("uptime_ms", heartbeat.Metrics.Uptime))
	}
}

// Handle page view
func handlePageView(conn *EnhancedAnalyticsConnection, message json.RawMessage) {
	conn.metrics.PageViews++
	conn.metrics.Events++

	var event struct {
		Page PageInfo `json:"page"`
	}

	if err := json.Unmarshal(message, &event); err == nil {
		conn.pagePath = event.Page.Path

		logger.Info("Page view tracked",
			slog.String("session", conn.sessionID),
			slog.String("path", event.Page.Path),
			slog.String("title", event.Page.Title))
	}
}

// Handle engagement metrics
func handleEngagement(conn *EnhancedAnalyticsConnection, message json.RawMessage) {
	conn.metrics.Events++

	var event struct {
		Metrics EngagementMetrics `json:"metrics"`
	}

	if err := json.Unmarshal(message, &event); err == nil {
		conn.metrics.Engagement = &event.Metrics

		logger.Debug("Engagement tracked",
			slog.String("session", conn.sessionID),
			slog.Int("time_on_page", event.Metrics.TimeOnPage),
			slog.Int("scroll_depth", event.Metrics.ScrollDepth),
			slog.Int("interactions", event.Metrics.Interactions))
	}
}

// Handle performance metrics
func handlePerformance(conn *EnhancedAnalyticsConnection, message json.RawMessage) {
	conn.metrics.Events++

	var event struct {
		Metrics PerformanceMetrics `json:"metrics"`
	}

	if err := json.Unmarshal(message, &event); err == nil {
		conn.metrics.Performance = &event.Metrics

		logger.Info("Performance tracked",
			slog.String("session", conn.sessionID),
			slog.Float64("load_time", event.Metrics.LoadTime),
			slog.Float64("fcp", event.Metrics.FirstContentfulPaint))
	}
}

// Handle custom events
func handleCustomEvent(conn *EnhancedAnalyticsConnection, message json.RawMessage) {
	conn.metrics.Events++

	var event struct {
		Name string      `json:"name"`
		Data interface{} `json:"data"`
	}

	if err := json.Unmarshal(message, &event); err == nil {
		logger.Debug("Custom event tracked",
			slog.String("session", conn.sessionID),
			slog.String("name", event.Name))
	}
}

// Handle disconnect
func handleDisconnect(conn *EnhancedAnalyticsConnection, message json.RawMessage) {
	var event struct {
		Reason string `json:"reason"`
	}

	if err := json.Unmarshal(message, &event); err == nil {
		logger.Info("Client disconnect",
			slog.String("session", conn.sessionID),
			slog.String("reason", event.Reason))
	}
}

// HandleBeacon handles fallback beacon API requests
func HandleBeacon(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	var payload BeaconPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	// Store beacon data for processing
	beaconBufferMutex.Lock()
	beaconBuffer = append(beaconBuffer, payload)

	// Prevent buffer overflow
	if len(beaconBuffer) > 1000 {
		beaconBuffer = beaconBuffer[100:] // Keep last 900 entries
	}
	beaconBufferMutex.Unlock()

	// Process events asynchronously
	go processBeaconEvents(payload)

	// Always return 204 No Content for beacon
	c.Status(http.StatusNoContent)
}

// Process beacon events
func processBeaconEvents(payload BeaconPayload) {
	logger.Info("Processing beacon events",
		slog.String("site", payload.Site),
		slog.String("session", payload.SessionID),
		slog.Int("events", len(payload.Events)))

	// Update session metrics if they exist
	sessionMetricsMutex.RLock()
	metrics, exists := sessionMetrics[payload.SessionID]
	sessionMetricsMutex.RUnlock()

	if exists {
		metrics.Events += len(payload.Events)
		metrics.LastActivity = time.Now()

		for _, event := range payload.Events {
			if event.Type == "pageview" {
				metrics.PageViews++
			}
		}
	}

	// Record in weekly data if manager exists
	if manager != nil {
		manager.recordVisitor(payload.Site, payload.SessionID)
	}
}

// GetEnhancedAnalytics returns enhanced analytics data with performance metrics
func GetEnhancedAnalytics() map[string]interface{} {
	combined := GetCombinedAnalytics()

	// Add session metrics
	sessionMetricsMutex.RLock()
	activeSessions := len(sessionMetrics)
	var totalEngagementTime int64
	var avgScrollDepth int
	var avgLoadTime float64
	var performanceCount int

	for _, metrics := range sessionMetrics {
		if metrics.Engagement != nil {
			totalEngagementTime += int64(metrics.Engagement.TimeOnPage)
			avgScrollDepth += metrics.Engagement.ScrollDepth
		}
		if metrics.Performance != nil {
			avgLoadTime += metrics.Performance.LoadTime
			performanceCount++
		}
	}
	sessionMetricsMutex.RUnlock()

	if activeSessions > 0 {
		avgScrollDepth = avgScrollDepth / activeSessions
	}
	if performanceCount > 0 {
		avgLoadTime = avgLoadTime / float64(performanceCount)
	}

	// Add beacon buffer size
	beaconBufferMutex.Lock()
	beaconQueueSize := len(beaconBuffer)
	beaconBufferMutex.Unlock()

	return map[string]interface{}{
		"sites": combined.Sites,
		"totals": map[string]interface{}{
			"active":       combined.TotalActive,
			"weekly":       combined.TotalWeekly,
			"active_sites": combined.ActiveSites,
		},
		"performance": map[string]interface{}{
			"active_sessions":    activeSessions,
			"avg_engagement_sec": totalEngagementTime / int64(max(activeSessions, 1)),
			"avg_scroll_depth":   avgScrollDepth,
			"avg_load_time_ms":   avgLoadTime,
			"beacon_queue":       beaconQueueSize,
		},
		"timestamp": combined.LastUpdated.Format(time.RFC3339),
	}
}

// CleanupSessionMetrics removes old session metrics
func CleanupSessionMetrics() {
	sessionMetricsMutex.Lock()
	defer sessionMetricsMutex.Unlock()

	cutoff := time.Now().Add(-24 * time.Hour)
	cleaned := 0

	for sessionID, metrics := range sessionMetrics {
		if metrics.LastActivity.Before(cutoff) {
			delete(sessionMetrics, sessionID)
			cleaned++
		}
	}

	if cleaned > 0 {
		logger.Info("Cleaned up session metrics",
			slog.Int("cleaned", cleaned),
			slog.Int("remaining", len(sessionMetrics)))
	}
}

// StartEnhancedCleanup Start cleanup routine for enhanced features
func StartEnhancedCleanup() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			CleanupSessionMetrics()

			// Clear old beacon buffer entries
			beaconBufferMutex.Lock()
			if len(beaconBuffer) > 500 {
				beaconBuffer = beaconBuffer[len(beaconBuffer)-500:]
			}
			beaconBufferMutex.Unlock()
		}
	}()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
