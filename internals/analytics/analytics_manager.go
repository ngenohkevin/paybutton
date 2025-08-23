package analytics

import (
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// WebSocket upgrader for analytics connections
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections from any origin for cross-site analytics
		origin := r.Header.Get("Origin")
		log.Printf("Analytics WebSocket connection attempt from origin: %s", origin)
		return true
	},
	EnableCompression: true,
}

// AnalyticsManager manages WebSocket connections and visitor tracking
type AnalyticsManager struct {
	// Real-time active connections
	connections map[string][]*AnalyticsConnection // siteName -> connections

	// Weekly visitor tracking (no database needed)
	weeklyData map[string]*SiteWeeklyData // siteName -> weekly stats

	// Phase 5: Historical data storage (30 days of hourly data)
	historicalData map[string]*SiteHistoricalData // siteName -> historical stats

	// Phase 5: Page path tracking
	pageData map[string]*SitePageData // siteName -> page analytics

	// Rate limiting
	rateLimiter map[string]*AnalyticsRateLimit // IP -> rate limit info

	mutex   sync.RWMutex
	cleanup chan string  // for connection cleanup
	ticker  *time.Ticker // hourly rotation
}

// AnalyticsRateLimit tracks connection attempts per IP
type AnalyticsRateLimit struct {
	attempts    int
	windowStart time.Time
	lastAccess  time.Time
}

// AnalyticsConnection represents an active WebSocket connection
type AnalyticsConnection struct {
	conn      *websocket.Conn
	sessionID string // unique session for weekly tracking
	siteName  string
	joinTime  time.Time
	pagePath  string // Phase 5: Track specific page path
	region    string // Phase 5: Geographic region (Tor-friendly)
}

// SiteWeeklyData tracks weekly visitor counts using rotating hourly buckets
type SiteWeeklyData struct {
	hourlyVisitors [168]int             // 24 hours * 7 days = 168 hours
	uniqueSessions map[string]time.Time // sessionID -> first visit time
	currentHour    int                  // rotating index (0-167)
	lastUpdate     time.Time
}

// WeeklyStats represents summary statistics for a site
type WeeklyStats struct {
	WeeklyTotal int       `json:"weekly_total"`
	LastSeen    time.Time `json:"last_seen"`
}

// SiteAnalytics represents combined analytics data for a site
type SiteAnalytics struct {
	SiteName    string    `json:"site_name"`
	ActiveCount int       `json:"active_count"`
	WeeklyTotal int       `json:"weekly_total"`
	LastSeen    time.Time `json:"last_seen"`
	// Phase 5: Additional fields
	TopPages []PageStats   `json:"top_pages,omitempty"`
	Regions  []RegionStats `json:"regions,omitempty"`
}

// Phase 5: Historical data storage (30 days of hourly data)
type SiteHistoricalData struct {
	hourlyData  [720]int       // 30 days * 24 hours = 720 hours
	timestamps  [720]time.Time // Corresponding timestamps
	currentHour int            // rotating index (0-719)
	lastUpdate  time.Time
}

// Phase 5: Page tracking for a site
type SitePageData struct {
	pageViews   map[string]*PageViewData // pagePath -> view data
	lastCleanup time.Time
}

// Phase 5: Individual page view tracking
type PageViewData struct {
	path           string
	viewCount      int
	uniqueSessions map[string]time.Time // sessionID -> last visit
	lastAccess     time.Time
}

// Phase 5: Popular page statistics
type PageStats struct {
	Path     string    `json:"path"`
	Views    int       `json:"views"`
	Unique   int       `json:"unique"`
	LastSeen time.Time `json:"last_seen"`
}

// Phase 5: Geographic region statistics
type RegionStats struct {
	Region string `json:"region"`
	Count  int    `json:"count"`
}

// Phase 5: Historical data point
type HistoricalDataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Viewers   int       `json:"viewers"`
}

// Phase 5: Export data structure
type SiteExportData struct {
	SiteName        string                `json:"site_name"`
	ExportTimestamp time.Time             `json:"export_timestamp"`
	Period          string                `json:"period"`
	Summary         SiteAnalytics         `json:"summary"`
	HistoricalData  []HistoricalDataPoint `json:"historical_data"`
	PageAnalytics   []PageStats           `json:"page_analytics"`
	RegionBreakdown []RegionStats         `json:"region_breakdown"`
}

// Global analytics manager instance
var manager *AnalyticsManager

// Global logger for analytics
var logger *slog.Logger

// Initialize creates and starts the analytics manager
func Initialize() {
	// Initialize structured logger for analytics
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With("component", "analytics")

	manager = &AnalyticsManager{
		connections:    make(map[string][]*AnalyticsConnection),
		weeklyData:     make(map[string]*SiteWeeklyData),
		historicalData: make(map[string]*SiteHistoricalData), // Phase 5
		pageData:       make(map[string]*SitePageData),       // Phase 5
		rateLimiter:    make(map[string]*AnalyticsRateLimit),
		cleanup:        make(chan string, 100),
		ticker:         time.NewTicker(time.Hour), // Rotate hourly data every hour
	}

	// Start background cleanup routines
	go manager.cleanupRoutine()
	go manager.hourlyRotation()

	logger.Info("Analytics Manager initialized",
		slog.Int("pid", os.Getpid()),
		slog.String("cleanup_interval", "60s"),
		slog.String("rotation_interval", "1h"))
}

// Shutdown gracefully closes all analytics connections and stops background routines
func Shutdown() {
	if manager == nil {
		return
	}

	logger.Info("Analytics Manager: Starting graceful shutdown")

	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	// Close all active connections
	totalConnections := 0
	siteCount := 0
	for siteName, connections := range manager.connections {
		siteCount++
		for _, conn := range connections {
			if conn.conn != nil {
				// Send close message to client
				closeMsg := map[string]interface{}{
					"status":    "shutdown",
					"reason":    "Server is shutting down",
					"timestamp": time.Now().Format(time.RFC3339),
				}

				if err := conn.conn.WriteJSON(closeMsg); err == nil {
					// Give client time to receive the message
					time.Sleep(100 * time.Millisecond)
				}

				conn.conn.Close()
				totalConnections++
			}
		}
		delete(manager.connections, siteName)
	}

	// Stop background routines
	if manager.ticker != nil {
		manager.ticker.Stop()
	}

	// Close cleanup channel
	close(manager.cleanup)

	logger.Info("Analytics Manager: Graceful shutdown completed",
		slog.Int("connections_closed", totalConnections),
		slog.Int("sites_affected", siteCount))
}

// HandleWebSocket handles WebSocket connections for analytics tracking
func HandleWebSocket(c *gin.Context) {
	siteName := c.Param("siteName")
	if siteName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "siteName parameter required"})
		return
	}

	// Phase 5: Extract page path and region info from query parameters and headers
	pagePath := c.Query("path")
	if pagePath == "" {
		pagePath = "/"
	}

	// Phase 5: Detect region using Tor-friendly method
	userAgent := c.GetHeader("User-Agent")
	acceptLanguage := c.GetHeader("Accept-Language")
	timezone := c.Query("tz") // Optional timezone from frontend
	region := detectTorFriendlyRegion(userAgent, acceptLanguage, timezone)

	// Note: Rate limiting removed to allow normal user browsing behavior
	// Users frequently open/close pages and this shouldn't be restricted
	clientIP := c.ClientIP()

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("Failed to upgrade analytics WebSocket",
			slog.String("site", siteName),
			slog.String("error", err.Error()),
			slog.String("remote_addr", c.ClientIP()))
		return
	}
	defer conn.Close()

	// Generate unique session ID (timestamp + random)
	sessionID := generateSessionID()

	// Add connection to manager with Phase 5 fields
	manager.addConnectionWithPath(siteName, conn, sessionID, pagePath, region)
	defer manager.removeConnection(siteName, sessionID)

	logger.Info("Analytics WebSocket connected",
		slog.String("site", siteName),
		slog.String("session", sessionID),
		slog.String("remote_addr", c.ClientIP()))

	// Send initial status
	initialStatus := map[string]interface{}{
		"status":    "connected",
		"site":      siteName,
		"sessionId": sessionID,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	if err := conn.WriteJSON(initialStatus); err != nil {
		logger.Error("Error sending initial analytics message",
			slog.String("site", siteName),
			slog.String("session", sessionID),
			slog.String("error", err.Error()))
		return
	}

	// Set read deadline for heartbeat detection (30 seconds timeout)
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	// Handle heartbeat messages
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		return nil
	})

	// Send ping every 15 seconds
	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	// Handle messages and heartbeat
	for {
		select {
		case <-pingTicker.C:
			if err := conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				logger.Warn("Analytics ping failed",
					slog.String("site", siteName),
					slog.String("session", sessionID),
					slog.String("error", err.Error()))
				return
			}
		default:
			// Read message from client (heartbeat or close)
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					logger.Warn("Analytics WebSocket unexpected close",
						slog.String("site", siteName),
						slog.String("session", sessionID),
						slog.String("error", err.Error()))
				}
				return
			}

			// Handle client heartbeat messages
			var clientMsg map[string]interface{}
			if err := json.Unmarshal(message, &clientMsg); err == nil {
				if clientMsg["type"] == "heartbeat" {
					// Reset read deadline on heartbeat
					conn.SetReadDeadline(time.Now().Add(30 * time.Second))
				}
			}
		}
	}
}

// addConnection adds a WebSocket connection for analytics tracking (legacy)
func (am *AnalyticsManager) addConnection(siteName string, conn *websocket.Conn, sessionID string) {
	am.addConnectionWithPath(siteName, conn, sessionID, "/", "Unknown")
}

// Phase 5: addConnectionWithPath adds a WebSocket connection with page path and region tracking
func (am *AnalyticsManager) addConnectionWithPath(siteName string, conn *websocket.Conn, sessionID, pagePath, region string) {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	// Initialize site connections if needed
	if am.connections[siteName] == nil {
		am.connections[siteName] = make([]*AnalyticsConnection, 0)
	}

	// Check memory limits per site (default: 1000 connections)
	maxConnectionsPerSite := 1000
	if len(am.connections[siteName]) >= maxConnectionsPerSite {
		logger.Warn("Site connection limit reached",
			slog.String("site", siteName),
			slog.Int("current_connections", len(am.connections[siteName])),
			slog.Int("limit", maxConnectionsPerSite))
		conn.Close()
		return
	}

	// Create connection record with Phase 5 fields
	analyticsConn := &AnalyticsConnection{
		conn:      conn,
		sessionID: sessionID,
		siteName:  siteName,
		joinTime:  time.Now(),
		pagePath:  pagePath, // Phase 5
		region:    region,   // Phase 5
	}

	// Add to active connections
	am.connections[siteName] = append(am.connections[siteName], analyticsConn)

	// Record visitor for weekly tracking
	am.recordVisitor(siteName, sessionID)

	// Phase 5: Record historical data
	am.recordHistoricalData(siteName)

	// Phase 5: Record page view
	am.recordPageView(siteName, pagePath, sessionID)

	logger.Info("Added analytics connection",
		slog.String("site", siteName),
		slog.String("session", sessionID),
		slog.String("page", pagePath),
		slog.String("region", region),
		slog.Int("site_active_count", len(am.connections[siteName])),
		slog.Int("total_sites", len(am.connections)))
}

// removeConnection removes a WebSocket connection
func (am *AnalyticsManager) removeConnection(siteName string, sessionID string) {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	connections := am.connections[siteName]
	for i, conn := range connections {
		if conn.sessionID == sessionID {
			// Remove connection from slice
			am.connections[siteName] = append(connections[:i], connections[i+1:]...)
			logger.Info("Removed analytics connection",
				slog.String("site", siteName),
				slog.String("session", sessionID),
				slog.Int("site_remaining", len(am.connections[siteName])))
			break
		}
	}

	// Clean up empty site entries
	if len(am.connections[siteName]) == 0 {
		delete(am.connections, siteName)
		logger.Info("Cleaned up empty connection list",
			slog.String("site", siteName))
	}
}

// recordVisitor records a visitor for weekly tracking
func (am *AnalyticsManager) recordVisitor(siteName string, sessionID string) {
	// Initialize site weekly data if needed
	if am.weeklyData[siteName] == nil {
		am.weeklyData[siteName] = &SiteWeeklyData{
			uniqueSessions: make(map[string]time.Time),
			currentHour:    getCurrentHourIndex(),
			lastUpdate:     time.Now(),
		}
	}

	siteData := am.weeklyData[siteName]
	now := time.Now()

	// Check if this is a new unique session for this hour
	if lastVisit, exists := siteData.uniqueSessions[sessionID]; !exists ||
		time.Since(lastVisit).Hours() >= 1 {

		// Update current hour index
		currentHour := getCurrentHourIndex()
		if currentHour != siteData.currentHour {
			// Hour has changed, rotate data if needed
			am.rotateHourlyDataForSite(siteName)
		}

		// Increment visitor count for current hour
		siteData.hourlyVisitors[siteData.currentHour]++

		// Record session visit time
		siteData.uniqueSessions[sessionID] = now
		siteData.lastUpdate = now

		logger.Info("Recorded unique visitor",
			slog.String("site", siteName),
			slog.String("session", sessionID),
			slog.Int("hourly_count", siteData.hourlyVisitors[siteData.currentHour]),
			slog.Int("hour_index", siteData.currentHour))
	}
}

// GetActiveViewers returns the current number of active viewers for a site
func (am *AnalyticsManager) GetActiveViewers(siteName string) int {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	return len(am.connections[siteName])
}

// GetAllSiteViewers returns active viewer counts for all sites
func (am *AnalyticsManager) GetAllSiteViewers() map[string]int {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	result := make(map[string]int)
	for siteName, connections := range am.connections {
		result[siteName] = len(connections)
	}

	return result
}

// GetWeeklyVisitors returns the total weekly visitor count for a site
func (am *AnalyticsManager) GetWeeklyVisitors(siteName string) int {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	siteData := am.weeklyData[siteName]
	if siteData == nil {
		return 0
	}

	// Sum all hourly visitors for the week
	total := 0
	for _, count := range siteData.hourlyVisitors {
		total += count
	}

	return total
}

// GetAllSiteWeeklyStats returns weekly statistics for all sites
func (am *AnalyticsManager) GetAllSiteWeeklyStats() map[string]WeeklyStats {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	result := make(map[string]WeeklyStats)
	for siteName, siteData := range am.weeklyData {
		// Calculate weekly total
		total := 0
		for _, count := range siteData.hourlyVisitors {
			total += count
		}

		result[siteName] = WeeklyStats{
			WeeklyTotal: total,
			LastSeen:    siteData.lastUpdate,
		}
	}

	return result
}

// GetSiteAnalytics returns combined analytics data for a site
func (am *AnalyticsManager) GetSiteAnalytics(siteName string) SiteAnalytics {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	activeCount := len(am.connections[siteName])
	weeklyTotal := 0
	lastSeen := time.Time{}

	if siteData := am.weeklyData[siteName]; siteData != nil {
		// Calculate weekly total
		for _, count := range siteData.hourlyVisitors {
			weeklyTotal += count
		}
		lastSeen = siteData.lastUpdate
	}

	return SiteAnalytics{
		SiteName:    siteName,
		ActiveCount: activeCount,
		WeeklyTotal: weeklyTotal,
		LastSeen:    lastSeen,
	}
}

// GetAllSiteAnalytics returns combined analytics for all sites
func GetAllSiteAnalytics() map[string]SiteAnalytics {
	if manager == nil {
		return make(map[string]SiteAnalytics)
	}

	manager.mutex.RLock()
	defer manager.mutex.RUnlock()

	result := make(map[string]SiteAnalytics)

	// Get all sites from both active connections and weekly data
	allSites := make(map[string]bool)
	for siteName := range manager.connections {
		allSites[siteName] = true
	}
	for siteName := range manager.weeklyData {
		allSites[siteName] = true
	}

	// Build analytics for each site
	for siteName := range allSites {
		result[siteName] = manager.GetSiteAnalytics(siteName)
	}

	return result
}

// rotateHourlyDataForSite rotates hourly data buckets for a specific site
func (am *AnalyticsManager) rotateHourlyDataForSite(siteName string) {
	siteData := am.weeklyData[siteName]
	if siteData == nil {
		return
	}

	newHour := getCurrentHourIndex()

	// If we've moved forward in time, clear old data
	if newHour != siteData.currentHour {
		hoursAdvanced := (newHour - siteData.currentHour + 168) % 168

		// Clear the buckets we're moving into
		for i := 0; i < hoursAdvanced && i < 168; i++ {
			bucketIndex := (siteData.currentHour + i + 1) % 168
			siteData.hourlyVisitors[bucketIndex] = 0
		}

		// Clean up old sessions (older than 7 days)
		cutoff := time.Now().Add(-7 * 24 * time.Hour)
		for sessionID, visitTime := range siteData.uniqueSessions {
			if visitTime.Before(cutoff) {
				delete(siteData.uniqueSessions, sessionID)
			}
		}

		siteData.currentHour = newHour
		logger.Info("Rotated hourly data",
			slog.String("site", siteName),
			slog.Int("new_hour_index", newHour),
			slog.Int("hours_advanced", hoursAdvanced),
			slog.Int("sessions_cleaned", len(siteData.uniqueSessions)))
	}
}

// cleanupRoutine runs periodic cleanup of stale connections
func (am *AnalyticsManager) cleanupRoutine() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			am.cleanupStaleConnections()
			am.cleanupRateLimits() // Clean up rate limit entries as well
		case siteName := <-am.cleanup:
			// Manual cleanup trigger
			am.cleanupStaleConnectionsForSite(siteName)
		}
	}
}

// cleanupStaleConnections removes dead connections
func (am *AnalyticsManager) cleanupStaleConnections() {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	for siteName := range am.connections {
		am.cleanupStaleConnectionsForSite(siteName)
	}
}

// cleanupStaleConnectionsForSite removes dead connections for a specific site
func (am *AnalyticsManager) cleanupStaleConnectionsForSite(siteName string) {
	connections := am.connections[siteName]
	activeConnections := make([]*AnalyticsConnection, 0)

	for _, conn := range connections {
		// Test connection with a ping
		if err := conn.conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
			logger.Warn("Removing stale analytics connection",
				slog.String("site", siteName),
				slog.String("session", conn.sessionID),
				slog.String("error", err.Error()))
			conn.conn.Close()
		} else {
			activeConnections = append(activeConnections, conn)
		}
	}

	if len(activeConnections) != len(connections) {
		staleCount := len(connections) - len(activeConnections)
		am.connections[siteName] = activeConnections
		if len(activeConnections) == 0 {
			delete(am.connections, siteName)
		}
		logger.Info("Cleaned up stale connections",
			slog.String("site", siteName),
			slog.Int("active_remaining", len(activeConnections)),
			slog.Int("stale_removed", staleCount))
	}
}

// hourlyRotation runs hourly data rotation for all sites
func (am *AnalyticsManager) hourlyRotation() {
	for range am.ticker.C {
		am.mutex.Lock()
		for siteName := range am.weeklyData {
			am.rotateHourlyDataForSite(siteName)
		}
		am.mutex.Unlock()
	}
}

// generateSessionID creates a unique session identifier
func generateSessionID() string {
	timestamp := time.Now().UnixNano()
	random := rand.Intn(10000)
	return fmt.Sprintf("%d_%d", timestamp, random)
}

// getCurrentHourIndex returns the current hour index (0-167) for the week
func getCurrentHourIndex() int {
	now := time.Now()
	// Calculate hours since start of week (Sunday = 0)
	weekday := int(now.Weekday())
	hour := now.Hour()
	return (weekday * 24) + hour
}

// Phase 5: getCurrentHistoricalHour returns the current hour index (0-719) for 30-day historical data
func getCurrentHistoricalHour() int {
	now := time.Now()
	// Get hours since epoch, then mod by 720 for 30-day rotation
	hoursFromEpoch := now.Unix() / 3600
	return int(hoursFromEpoch % 720)
}

// Phase 5: detectTorFriendlyRegion detects geographic region without using IP geolocation
// This is Tor-friendly as it doesn't rely on IP addresses for location
func detectTorFriendlyRegion(userAgent, acceptLanguage, timezone string) string {
	// Use Accept-Language header as primary indicator (Tor-friendly)
	if acceptLanguage != "" {
		// Extract primary language from Accept-Language
		languages := strings.Split(acceptLanguage, ",")
		if len(languages) > 0 {
			primaryLang := strings.TrimSpace(strings.Split(languages[0], ";")[0])

			// Map languages to general regions (privacy-friendly)
			switch {
			case strings.HasPrefix(primaryLang, "en"):
				return "English-speaking"
			case strings.HasPrefix(primaryLang, "es"):
				return "Spanish-speaking"
			case strings.HasPrefix(primaryLang, "fr"):
				return "French-speaking"
			case strings.HasPrefix(primaryLang, "de"):
				return "German-speaking"
			case strings.HasPrefix(primaryLang, "it"):
				return "Italian-speaking"
			case strings.HasPrefix(primaryLang, "pt"):
				return "Portuguese-speaking"
			case strings.HasPrefix(primaryLang, "ru"):
				return "Russian-speaking"
			case strings.HasPrefix(primaryLang, "zh"):
				return "Chinese-speaking"
			case strings.HasPrefix(primaryLang, "ja"):
				return "Japanese-speaking"
			case strings.HasPrefix(primaryLang, "ko"):
				return "Korean-speaking"
			case strings.HasPrefix(primaryLang, "ar"):
				return "Arabic-speaking"
			case strings.HasPrefix(primaryLang, "hi"):
				return "Hindi-speaking"
			default:
				return "Other"
			}
		}
	}

	// Fallback to "Unknown" for maximum privacy
	return "Unknown"
}

// GetTotalActiveViewers returns the total number of active viewers across all sites
func GetTotalActiveViewers() int {
	if manager == nil {
		return 0
	}

	manager.mutex.RLock()
	defer manager.mutex.RUnlock()

	total := 0
	for _, connections := range manager.connections {
		total += len(connections)
	}

	return total
}

// GetTotalWeeklyVisitors returns the total weekly visitors across all sites
func GetTotalWeeklyVisitors() int {
	if manager == nil {
		return 0
	}

	manager.mutex.RLock()
	defer manager.mutex.RUnlock()

	total := 0
	for _, siteData := range manager.weeklyData {
		for _, count := range siteData.hourlyVisitors {
			total += count
		}
	}

	return total
}

// GetActiveSitesCount returns the number of sites with active connections
func GetActiveSitesCount() int {
	if manager == nil {
		return 0
	}

	manager.mutex.RLock()
	defer manager.mutex.RUnlock()

	return len(manager.connections)
}

// Phase 5: recordHistoricalData records hourly visitor count data
func (am *AnalyticsManager) recordHistoricalData(siteName string) {
	// Initialize site historical data if needed
	if am.historicalData[siteName] == nil {
		am.historicalData[siteName] = &SiteHistoricalData{
			currentHour: getCurrentHistoricalHour(),
			lastUpdate:  time.Now(),
		}

		// Initialize timestamps for the circular buffer
		now := time.Now()
		for i := 0; i < 720; i++ {
			am.historicalData[siteName].timestamps[i] = now.Add(time.Duration(-719+i) * time.Hour)
		}
	}

	siteData := am.historicalData[siteName]
	now := time.Now()
	currentHour := getCurrentHistoricalHour()

	// Rotate data if hour has changed
	if currentHour != siteData.currentHour {
		// Clear hours between last update and current hour
		hoursAdvanced := (currentHour - siteData.currentHour + 720) % 720
		for i := 0; i < hoursAdvanced && i < 720; i++ {
			bucketIndex := (siteData.currentHour + i + 1) % 720
			siteData.hourlyData[bucketIndex] = 0
			siteData.timestamps[bucketIndex] = now.Add(time.Duration(bucketIndex-currentHour) * time.Hour)
		}
		siteData.currentHour = currentHour
	}

	// Increment current hour's visitor count
	siteData.hourlyData[currentHour] = len(am.connections[siteName])
	siteData.timestamps[currentHour] = now
	siteData.lastUpdate = now
}

// Phase 5: recordPageView records page view data
func (am *AnalyticsManager) recordPageView(siteName, pagePath, sessionID string) {
	// Initialize site page data if needed
	if am.pageData[siteName] == nil {
		am.pageData[siteName] = &SitePageData{
			pageViews:   make(map[string]*PageViewData),
			lastCleanup: time.Now(),
		}
	}

	sitePageData := am.pageData[siteName]

	// Initialize page data if needed
	if sitePageData.pageViews[pagePath] == nil {
		sitePageData.pageViews[pagePath] = &PageViewData{
			path:           pagePath,
			uniqueSessions: make(map[string]time.Time),
			lastAccess:     time.Now(),
		}
	}

	pageData := sitePageData.pageViews[pagePath]
	now := time.Now()

	// Check if this is a new unique session for this page (within last hour)
	if lastVisit, exists := pageData.uniqueSessions[sessionID]; !exists ||
		time.Since(lastVisit).Hours() >= 1 {
		pageData.uniqueSessions[sessionID] = now
	}

	pageData.viewCount++
	pageData.lastAccess = now

	// Cleanup old page data every 24 hours
	if time.Since(sitePageData.lastCleanup).Hours() >= 24 {
		am.cleanupOldPageData(siteName)
		sitePageData.lastCleanup = now
	}
}

// Phase 5: cleanupOldPageData removes old session data from page views
func (am *AnalyticsManager) cleanupOldPageData(siteName string) {
	sitePageData := am.pageData[siteName]
	if sitePageData == nil {
		return
	}

	cutoff := time.Now().Add(-7 * 24 * time.Hour) // Keep 7 days of data

	for pagePath, pageData := range sitePageData.pageViews {
		// Clean up old sessions
		for sessionID, visitTime := range pageData.uniqueSessions {
			if visitTime.Before(cutoff) {
				delete(pageData.uniqueSessions, sessionID)
			}
		}

		// Remove pages with no recent activity
		if pageData.lastAccess.Before(cutoff) {
			delete(sitePageData.pageViews, pagePath)
		}
	}
}

// Phase 5: GetSiteHistoricalData returns historical data for a site
func (am *AnalyticsManager) GetSiteHistoricalData(siteName string, hours int) []HistoricalDataPoint {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	siteData := am.historicalData[siteName]
	if siteData == nil {
		return []HistoricalDataPoint{}
	}

	// Limit hours to available data (max 720)
	if hours > 720 || hours <= 0 {
		hours = 720
	}

	result := make([]HistoricalDataPoint, 0, hours)
	currentHour := siteData.currentHour

	// Get last N hours of data
	for i := hours - 1; i >= 0; i-- {
		bucketIndex := (currentHour - i + 720) % 720
		result = append(result, HistoricalDataPoint{
			Timestamp: siteData.timestamps[bucketIndex],
			Viewers:   siteData.hourlyData[bucketIndex],
		})
	}

	return result
}

// Phase 5: GetSitePageStats returns popular page statistics for a site
func (am *AnalyticsManager) GetSitePageStats(siteName string, limit int) []PageStats {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	sitePageData := am.pageData[siteName]
	if sitePageData == nil {
		return []PageStats{}
	}

	// Build page stats
	pageStats := make([]PageStats, 0, len(sitePageData.pageViews))
	for _, pageData := range sitePageData.pageViews {
		pageStats = append(pageStats, PageStats{
			Path:     pageData.path,
			Views:    pageData.viewCount,
			Unique:   len(pageData.uniqueSessions),
			LastSeen: pageData.lastAccess,
		})
	}

	// Sort by view count (descending)
	for i := 0; i < len(pageStats)-1; i++ {
		for j := i + 1; j < len(pageStats); j++ {
			if pageStats[i].Views < pageStats[j].Views {
				pageStats[i], pageStats[j] = pageStats[j], pageStats[i]
			}
		}
	}

	// Limit results
	if limit > 0 && len(pageStats) > limit {
		pageStats = pageStats[:limit]
	}

	return pageStats
}

// Phase 5: GetSiteRegionStats returns region statistics for a site
func (am *AnalyticsManager) GetSiteRegionStats(siteName string) []RegionStats {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	connections := am.connections[siteName]
	if len(connections) == 0 {
		return []RegionStats{}
	}

	// Count connections by region
	regionCounts := make(map[string]int)
	for _, conn := range connections {
		regionCounts[conn.region]++
	}

	// Convert to slice and sort
	regionStats := make([]RegionStats, 0, len(regionCounts))
	for region, count := range regionCounts {
		regionStats = append(regionStats, RegionStats{
			Region: region,
			Count:  count,
		})
	}

	// Sort by count (descending)
	for i := 0; i < len(regionStats)-1; i++ {
		for j := i + 1; j < len(regionStats); j++ {
			if regionStats[i].Count < regionStats[j].Count {
				regionStats[i], regionStats[j] = regionStats[j], regionStats[i]
			}
		}
	}

	return regionStats
}

// Phase 5: ExportSiteData returns comprehensive export data for a site
func ExportSiteData(siteName string, period string) *SiteExportData {
	if manager == nil {
		return nil
	}

	manager.mutex.RLock()
	defer manager.mutex.RUnlock()

	// Get hours based on period
	hours := 720 // Default: 30 days
	switch period {
	case "24h":
		hours = 24
	case "7d":
		hours = 168
	case "30d":
		hours = 720
	}

	// Get all data
	summary := manager.GetSiteAnalytics(siteName)
	summary.TopPages = manager.GetSitePageStats(siteName, 10)
	summary.Regions = manager.GetSiteRegionStats(siteName)

	historicalData := manager.GetSiteHistoricalData(siteName, hours)
	pageStats := manager.GetSitePageStats(siteName, 0) // All pages
	regionStats := manager.GetSiteRegionStats(siteName)

	return &SiteExportData{
		SiteName:        siteName,
		ExportTimestamp: time.Now(),
		Period:          period,
		Summary:         summary,
		HistoricalData:  historicalData,
		PageAnalytics:   pageStats,
		RegionBreakdown: regionStats,
	}
}

// checkRateLimit checks if the IP is within rate limits for WebSocket connections
func (am *AnalyticsManager) checkRateLimit(clientIP string) bool {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	now := time.Now()
	windowDuration := 5 * time.Minute // 5-minute window
	maxAttempts := 100                // Max 100 connections per 5 minutes (increased for testing)

	// Get or create rate limit record for this IP
	rateLimit, exists := am.rateLimiter[clientIP]
	if !exists {
		am.rateLimiter[clientIP] = &AnalyticsRateLimit{
			attempts:    1,
			windowStart: now,
			lastAccess:  now,
		}
		return true
	}

	// Update last access
	rateLimit.lastAccess = now

	// Check if we need to reset the window
	if now.Sub(rateLimit.windowStart) > windowDuration {
		rateLimit.attempts = 1
		rateLimit.windowStart = now
		return true
	}

	// Check if within limits
	if rateLimit.attempts >= maxAttempts {
		logger.Warn("Rate limit exceeded",
			slog.String("client_ip", clientIP),
			slog.Int("attempts", rateLimit.attempts),
			slog.Duration("window_age", now.Sub(rateLimit.windowStart)))
		return false
	}

	// Increment attempts
	rateLimit.attempts++
	return true
}

// cleanupRateLimits removes stale rate limit entries
func (am *AnalyticsManager) cleanupRateLimits() {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	now := time.Now()
	staleTimeout := 1 * time.Hour // Remove entries older than 1 hour

	cleaned := 0
	for ip, rateLimit := range am.rateLimiter {
		if now.Sub(rateLimit.lastAccess) > staleTimeout {
			delete(am.rateLimiter, ip)
			cleaned++
		}
	}

	if cleaned > 0 {
		logger.Info("Cleaned up stale rate limit entries",
			slog.Int("cleaned", cleaned),
			slog.Int("remaining", len(am.rateLimiter)))
	}
}

// Phase 5: Public functions for API access
func GetSiteHistoricalData(siteName string, hours int) []HistoricalDataPoint {
	if manager == nil {
		return []HistoricalDataPoint{}
	}
	return manager.GetSiteHistoricalData(siteName, hours)
}

func GetSitePageStats(siteName string, limit int) []PageStats {
	if manager == nil {
		return []PageStats{}
	}
	return manager.GetSitePageStats(siteName, limit)
}

func GetSiteRegionStats(siteName string) []RegionStats {
	if manager == nil {
		return []RegionStats{}
	}
	return manager.GetSiteRegionStats(siteName)
}
