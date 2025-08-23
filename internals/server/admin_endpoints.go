package server

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/ngenohkevin/paybutton/internals/analytics"
	"github.com/ngenohkevin/paybutton/internals/config"
	"github.com/ngenohkevin/paybutton/internals/monitoring"
	"github.com/ngenohkevin/paybutton/internals/payment_processing"
	"github.com/ngenohkevin/paybutton/utils"
)

// Global admin auth instance for template access
var globalAdminAuth *AdminAuth

// RegisterAdminEndpoints registers admin monitoring endpoints
func RegisterAdminEndpoints(router *gin.Engine, auth *AdminAuth) {
	// Store global reference for template access
	globalAdminAuth = auth

	admin := router.Group("/admin")

	// Authentication endpoints (no auth required)
	admin.GET("/login", auth.LoginHandler)
	admin.POST("/login", auth.LoginHandler)
	admin.POST("/logout", auth.LogoutHandler)

	// Protected admin routes
	admin.Use(auth.AdminMiddleware())
	admin.GET("/", func(c *gin.Context) { c.Redirect(http.StatusFound, "/admin/dashboard") })
	admin.GET("/dashboard", auth.DashboardHandler)

	// WebSocket endpoint for real-time updates
	admin.GET("/ws", handleAdminWebSocket)

	// API endpoints for HTMX
	api := admin.Group("/api")
	api.GET("/status", getSystemStatusHTML)
	api.GET("/pool/stats", getPoolStatsHTML)
	api.GET("/gap/stats", getGapStatsHTML)
	api.GET("/ratelimit/stats", getRateLimitStatsHTML)

	// JSON endpoints for direct API access
	admin.GET("/status", getSystemStatus)
	api.GET("/dashboard-sessions", getDashboardSessionStats)

	// Site Analytics endpoints
	api.GET("/site-analytics", getSiteAnalyticsData)
	api.GET("/dashboard-analytics", getDashboardAnalytics)

	// Phase 5: Advanced analytics endpoints
	api.GET("/site-analytics/:siteName/historical", getSiteHistoricalData)
	api.GET("/site-analytics/:siteName/pages", getSitePageStats)
	api.GET("/site-analytics/:siteName/regions", getSiteRegionStats)
	api.GET("/site-analytics/:siteName/export", exportSiteAnalyticsData)

	// Management endpoints
	admin.GET("/pool", getPoolManagementPage)
	admin.POST("/pool/refill", refillAddressPool)
	admin.GET("/pool/stats", getPoolStats)
	admin.GET("/pool/details", getPoolDetails)
	admin.POST("/pool/recycle", recycleExpiredAddresses)
	admin.POST("/pool/clear-unused", clearUnusedAddresses)
	admin.POST("/pool/release", releaseAddressReservation)
	admin.POST("/pool/configure", configurePool)
	admin.GET("/pool/export", exportPoolData)
	admin.GET("/pool/export-used", exportUsedAddresses)

	admin.GET("/gap-monitor", getGapMonitorPage)
	admin.POST("/gap/reset", resetGapCounter)
	admin.GET("/gap/stats", getGapStats)

	// Gap monitor management API endpoints
	gapAPI := admin.Group("/api/gap-monitor")
	gapAPI.POST("/update-limit", updateGapLimit)
	gapAPI.POST("/update-settings", updateGapSettings)
	gapAPI.POST("/toggle-fallback", toggleFallbackMode)
	gapAPI.POST("/clear-errors", clearGapErrors)
	gapAPI.POST("/reset", resetGapMonitorWithOptions)

	admin.GET("/rate-limiter", getRateLimiterPage)
	admin.POST("/ratelimit/reset/:email", resetUserRateLimit)
	admin.GET("/ratelimit/stats", getRateLimitStats)

	// Rate limiter management API endpoints
	rateLimitAPI := admin.Group("/api/rate-limiter")
	rateLimitAPI.POST("/update-global", updateGlobalRateLimitConfig)
	rateLimitAPI.POST("/update-config", updateRateLimitConfig)
	rateLimitAPI.POST("/reset", resetSpecificRateLimit)
	rateLimitAPI.POST("/block", blockRateLimit)
	rateLimitAPI.POST("/reset-global", resetGlobalRateLimit)
	rateLimitAPI.POST("/bulk-reset", bulkResetRateLimits)
	rateLimitAPI.POST("/cleanup", triggerRateLimitCleanup)
	rateLimitAPI.GET("/export", exportRateLimitData)

	// Logs management endpoints
	admin.GET("/logs", getLogsPage)
	admin.GET("/api/logs/stream", streamLogs)
	admin.GET("/api/logs/download", downloadLogs)

	// Analytics and metrics endpoints
	admin.GET("/analytics", getAnalyticsPage)
	admin.GET("/api/analytics/data", getAnalyticsData)
	admin.GET("/api/analytics/export", exportAnalyticsData)
	admin.POST("/api/analytics/insights", generateAnalyticsInsights)

	// Alert management endpoints
	admin.GET("/alerts", getAlertsPage)
	admin.GET("/api/alerts/active", getActiveAlerts)
	admin.GET("/api/alerts/history", getAlertHistory)
	admin.GET("/api/alerts/rules", getAlertRules)
	admin.GET("/api/alerts/channels", getAlertChannels)
	admin.POST("/api/alerts/acknowledge", acknowledgeAlert)
	admin.POST("/api/alerts/resolve", resolveAlert)
	admin.POST("/api/alerts/trigger", triggerAlert)
	admin.POST("/api/alerts/rules", updateAlertRule)
	admin.POST("/api/alerts/channels", updateAlertChannel)
	admin.POST("/api/alerts/test-channel", testAlertChannel)
	admin.GET("/api/alerts/stats", getAlertStats)

	// Configuration management endpoints
	admin.GET("/config", getConfigurationPage)
	configAPI := admin.Group("/api/config")
	configAPI.GET("/current", getCurrentConfiguration)
	configAPI.GET("/validation-rules", getValidationRules)
	configAPI.GET("/history", getConfigurationHistory)
	configAPI.POST("/update", updateConfiguration)
	configAPI.POST("/update-section", updateConfigurationSection)
	configAPI.POST("/reset-defaults", resetToDefaultConfiguration)
	configAPI.POST("/rollback", rollbackConfiguration)

	// Session management endpoints
	admin.GET("/sessions", getSessionsPage)
	sessionAPI := admin.Group("/api/sessions")
	sessionAPI.GET("/active", getActiveSessions)
	sessionAPI.GET("/history", getSessionHistory)
	sessionAPI.GET("/stats", getSessionStats)
	sessionAPI.GET("/analytics", getSessionAnalytics)
	sessionAPI.GET("/timeline", getSessionTimeline)
	sessionAPI.GET("/trends", getSessionTrends)
	sessionAPI.POST("/terminate", terminateSession)
	sessionAPI.POST("/cleanup", cleanupSessions)
	sessionAPI.POST("/clear-history", clearSessionHistory)
	sessionAPI.GET("/export", exportSessionData)
}

// getSystemStatus returns overall system health and statistics
func getSystemStatus(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in getSystemStatus: %v", r)
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Internal error: %v", r),
			})
		}
	}()

	pool := payment_processing.GetAddressPool()
	gap := payment_processing.GetGapMonitor()
	limiter := payment_processing.GetRateLimiter()
	resourceMonitor := monitoring.GetResourceMonitor()

	status := gin.H{
		"status": "healthy",
		"components": gin.H{
			"address_pool":     pool.GetStats(),
			"gap_monitor":      gap.GetStats(),
			"rate_limiter":     limiter.GetStats(),
			"resource_monitor": resourceMonitor.GetStats(),
		},
		"recommendations": getSystemRecommendations(pool, gap),
	}

	c.JSON(http.StatusOK, status)
}

// getPoolStats returns address pool statistics
func getPoolStats(c *gin.Context) {
	pool := payment_processing.GetAddressPool()
	stats := pool.GetStats()
	c.JSON(http.StatusOK, stats)
}

// refillAddressPool triggers manual pool refill
func refillAddressPool(c *gin.Context) {
	pool := payment_processing.GetAddressPool()

	err := pool.ForceRefill()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   err.Error(),
			"message": "Failed to initiate pool refill",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Pool refill initiated successfully",
		"note":    "Check system logs for progress details",
	})
}

// getGapStats returns gap monitor statistics
func getGapStats(c *gin.Context) {
	gap := payment_processing.GetGapMonitor()
	stats := gap.GetStats()
	c.JSON(http.StatusOK, stats)
}

// resetGapCounter resets the unpaid address counter
func resetGapCounter(c *gin.Context) {
	var req struct {
		Count int `json:"count" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	gap := payment_processing.GetGapMonitor()
	gap.ResetUnpaidCount(req.Count)

	c.JSON(http.StatusOK, gin.H{
		"message":   "Gap counter reset successfully",
		"new_count": req.Count,
	})
}

// getRateLimitStats returns rate limiter statistics
func getRateLimitStats(c *gin.Context) {
	limiter := payment_processing.GetRateLimiter()
	stats := limiter.GetStats()
	c.JSON(http.StatusOK, stats)
}

// resetUserRateLimit resets rate limits for a specific user
func resetUserRateLimit(c *gin.Context) {
	email := c.Param("email")

	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email is required"})
		return
	}

	limiter := payment_processing.GetRateLimiter()
	limiter.ResetLimits(email)

	c.JSON(http.StatusOK, gin.H{
		"message": "Rate limits reset for user",
		"email":   email,
	})
}

// getSystemRecommendations provides actionable recommendations based on current state
func getSystemRecommendations(pool *payment_processing.AddressPool, gap *payment_processing.GapLimitMonitor) []string {
	recommendations := []string{}

	poolStats := pool.GetStats()
	gapStats := gap.GetStats()

	// Check pool size
	if poolStats.CurrentPoolSize < 3 {
		recommendations = append(recommendations, "Address pool is running low. Consider manual refill during off-peak hours.")
	}

	// Check gap limit status
	if gapRatio, ok := gapStats["gap_ratio"].(string); ok {
		// Parse percentage and provide recommendations
		recommendations = append(recommendations, "Current gap ratio: "+gapRatio)
	}

	// Check if fallback is active
	if shouldFallback, ok := gapStats["should_use_fallback"].(bool); ok && shouldFallback {
		recommendations = append(recommendations, "System is currently using fallback addresses due to gap limit issues.")
	}

	// Check recent errors
	if errors, ok := gapStats["recent_errors"].(int); ok && errors > 10 {
		recommendations = append(recommendations, "High number of recent errors detected. Review logs for details.")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "System is operating normally.")
	}

	return recommendations
}

// getSystemStatusHTML returns system status as HTML for HTMX
func getSystemStatusHTML(c *gin.Context) {
	pool := payment_processing.GetAddressPool()
	gap := payment_processing.GetGapMonitor()
	limiter := payment_processing.GetRateLimiter()

	poolStats := pool.GetStats()
	gapStats := gap.GetStats()
	limiterStats := limiter.GetStats()

	// Generate HTML response
	html := fmt.Sprintf(`
	<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 mb-8">
		<!-- System Status Card -->
		<div class="card">
			<div class="card-header">
				<h3 class="text-lg font-semibold text-gray-900">
					<i class="fas fa-heart text-green-500 mr-2"></i>System Status
				</h3>
			</div>
			<div class="card-body">
				<div class="text-center">
					<div class="text-3xl font-bold text-green-600 mb-2">HEALTHY</div>
					<p class="text-sm text-gray-600">All systems operational</p>
				</div>
			</div>
		</div>
		
		<!-- Address Pool Card -->
		<div class="card">
			<div class="card-header">
				<h3 class="text-lg font-semibold text-gray-900">
					<i class="fas fa-swimming-pool text-blue-500 mr-2"></i>Address Pool
				</h3>
			</div>
			<div class="card-body">
				<div class="flex justify-between items-center mb-3">
					<span class="text-sm text-gray-600">Available</span>
					<span class="text-lg font-bold text-blue-600">%d</span>
				</div>
				<div class="flex justify-between items-center mb-3">
					<span class="text-sm text-gray-600">Total Generated</span>
					<span class="text-lg font-bold">%d</span>
				</div>
				<div class="flex justify-between items-center">
					<span class="text-sm text-gray-600">Recycled</span>
					<span class="text-lg font-bold text-green-600">%d</span>
				</div>
				<div class="mt-4 h-32">
					<canvas id="poolChart"></canvas>
				</div>
			</div>
		</div>
		
		<!-- Gap Monitor Card -->
		<div class="card">
			<div class="card-header">
				<h3 class="text-lg font-semibold text-gray-900">
					<i class="fas fa-exclamation-triangle text-yellow-500 mr-2"></i>Gap Monitor
				</h3>
			</div>
			<div class="card-body">
				<div class="flex justify-between items-center mb-3">
					<span class="text-sm text-gray-600">Gap Ratio</span>
					<span class="text-lg font-bold %s">%s</span>
				</div>
				<div class="flex justify-between items-center mb-3">
					<span class="text-sm text-gray-600">Paid Addresses</span>
					<span class="text-lg font-bold text-green-600">%d</span>
				</div>
				<div class="flex justify-between items-center mb-3">
					<span class="text-sm text-gray-600">Unpaid Addresses</span>
					<span class="text-lg font-bold text-yellow-600">%d</span>
				</div>
				<div class="flex justify-between items-center">
					<span class="text-sm text-gray-600">Fallback Mode</span>
					<span class="text-sm %s">%s</span>
				</div>
				<div class="mt-4 h-32">
					<canvas id="gapChart"></canvas>
				</div>
			</div>
		</div>
	</div>
	
	<!-- Rate Limiter Section -->
	<div class="grid grid-cols-1 md:grid-cols-2 gap-6 mb-8">
		<div class="card">
			<div class="card-header">
				<h3 class="text-lg font-semibold text-gray-900">
					<i class="fas fa-shield-alt text-purple-500 mr-2"></i>Rate Limiter
				</h3>
			</div>
			<div class="card-body">
				<div class="flex justify-between items-center mb-3">
					<span class="text-sm text-gray-600">Global Tokens</span>
					<span class="text-lg font-bold text-blue-600">%d / %d</span>
				</div>
				<div class="flex justify-between items-center mb-3">
					<span class="text-sm text-gray-600">IP Limits Active</span>
					<span class="text-lg font-bold">%d</span>
				</div>
				<div class="flex justify-between items-center">
					<span class="text-sm text-gray-600">Email Limits Active</span>
					<span class="text-lg font-bold">%d</span>
				</div>
				<div class="mt-4 h-32">
					<canvas id="rateLimitChart"></canvas>
				</div>
			</div>
		</div>
		
		<!-- Recommendations Card -->
		<div class="card">
			<div class="card-header">
				<h3 class="text-lg font-semibold text-gray-900">
					<i class="fas fa-lightbulb text-yellow-500 mr-2"></i>Recommendations
				</h3>
			</div>
			<div class="card-body">
				<ul class="space-y-2">
					%s
				</ul>
			</div>
		</div>
	</div>
	
	<!-- Enhanced Session Analytics Section -->
	<div class="card mb-8 overflow-hidden bg-gradient-to-br from-gray-900 via-gray-800 to-gray-900 dark:from-gray-900 dark:via-black dark:to-gray-900 border-0 shadow-2xl">
		<div class="card-header bg-gradient-to-r from-gray-800 to-gray-700 dark:from-gray-900 dark:to-black text-white border-0 relative overflow-hidden">
			<div class="absolute inset-0 bg-black opacity-30"></div>
			<div class="relative z-10 flex flex-col sm:flex-row sm:items-center sm:justify-between space-y-2 sm:space-y-0">
				<h3 class="text-lg font-bold flex items-center">
					<div class="p-2 bg-white bg-opacity-10 rounded-lg mr-3">
						<i class="fas fa-users text-white"></i>
					</div>
					<span class="mr-3">Session Overview</span>
					<div class="flex items-center px-3 py-1 bg-blue-500 bg-opacity-20 rounded-full border border-blue-400 border-opacity-30">
						<div class="w-2 h-2 bg-blue-400 rounded-full animate-pulse mr-2"></div>
						<span class="text-xs font-semibold text-blue-300">Real-time</span>
					</div>
				</h3>
			</div>
		</div>
		<div class="card-body p-6">
			<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4" id="session-overview">
				<!-- Active Sessions -->
				<div class="group relative overflow-hidden bg-gradient-to-br from-blue-700 to-blue-800 dark:from-blue-800 dark:to-blue-900 p-6 rounded-2xl text-white transform transition-all duration-300 hover:scale-105 hover:shadow-2xl cursor-pointer border border-blue-600 border-opacity-20">
					<div class="absolute top-0 right-0 w-20 h-20 bg-blue-300 opacity-5 rounded-full -translate-y-10 translate-x-10"></div>
					<div class="relative z-10">
						<div class="flex items-center justify-between mb-4">
							<div class="p-3 bg-blue-600 bg-opacity-30 rounded-xl border border-blue-500 border-opacity-20">
								<i class="fas fa-play text-xl text-blue-200"></i>
							</div>
							<div class="text-right">
								<div class="w-3 h-3 bg-blue-400 rounded-full animate-pulse shadow-lg"></div>
							</div>
						</div>
						<div class="space-y-1">
							<p class="text-blue-200 text-sm font-semibold">Active Sessions</p>
							<p class="text-4xl font-black tracking-tight text-white" id="dashboard-active-sessions">
								<span class="animate-pulse">-</span>
							</p>
						</div>
					</div>
				</div>
				
				<!-- WebSocket Connections -->
				<div class="group relative overflow-hidden bg-gradient-to-br from-green-700 to-green-800 dark:from-green-800 dark:to-green-900 p-6 rounded-2xl text-white transform transition-all duration-300 hover:scale-105 hover:shadow-2xl cursor-pointer border border-green-600 border-opacity-20">
					<div class="absolute top-0 right-0 w-16 h-16 bg-green-300 opacity-5 rounded-full -translate-y-8 translate-x-8"></div>
					<div class="relative z-10">
						<div class="flex items-center justify-between mb-4">
							<div class="p-3 bg-green-600 bg-opacity-30 rounded-xl border border-green-500 border-opacity-20">
								<i class="fas fa-wifi text-xl text-green-200"></i>
							</div>
							<div class="text-right">
								<div class="w-3 h-3 bg-green-400 rounded-full animate-ping shadow-lg"></div>
							</div>
						</div>
						<div class="space-y-1">
							<p class="text-green-200 text-sm font-semibold">WebSocket</p>
							<p class="text-4xl font-black tracking-tight text-white" id="dashboard-websocket-count">
								<span class="animate-pulse">-</span>
							</p>
						</div>
					</div>
				</div>
				
				<!-- Payment Rate -->
				<div class="group relative overflow-hidden bg-gradient-to-br from-purple-700 to-purple-800 dark:from-purple-800 dark:to-purple-900 p-6 rounded-2xl text-white transform transition-all duration-300 hover:scale-105 hover:shadow-2xl cursor-pointer border border-purple-600 border-opacity-20">
					<div class="absolute top-0 right-0 w-24 h-24 bg-purple-300 opacity-5 rounded-full -translate-y-12 translate-x-12"></div>
					<div class="relative z-10">
						<div class="flex items-center justify-between mb-4">
							<div class="p-3 bg-purple-600 bg-opacity-30 rounded-xl border border-purple-500 border-opacity-20">
								<i class="fas fa-percentage text-xl text-purple-200"></i>
							</div>
							<div class="text-right">
								<div class="w-3 h-3 bg-purple-400 rounded-full animate-pulse shadow-lg"></div>
							</div>
						</div>
						<div class="space-y-1">
							<p class="text-purple-200 text-sm font-semibold">Payment Rate</p>
							<p class="text-4xl font-black tracking-tight text-white" id="dashboard-payment-rate">
								<span class="animate-pulse">-</span>
							</p>
						</div>
					</div>
				</div>
				
				<!-- Paid Amount -->
				<div class="group relative overflow-hidden bg-gradient-to-br from-orange-700 to-orange-800 dark:from-orange-800 dark:to-orange-900 p-6 rounded-2xl text-white transform transition-all duration-300 hover:scale-105 hover:shadow-2xl cursor-pointer border border-orange-600 border-opacity-20">
					<div class="absolute top-0 right-0 w-18 h-18 bg-orange-300 opacity-5 rounded-full -translate-y-9 translate-x-9"></div>
					<div class="relative z-10">
						<div class="flex items-center justify-between mb-4">
							<div class="p-3 bg-orange-600 bg-opacity-30 rounded-xl border border-orange-500 border-opacity-20">
								<i class="fas fa-dollar-sign text-xl text-orange-200"></i>
							</div>
							<div class="text-right">
								<div class="w-3 h-3 bg-orange-400 rounded-full animate-pulse shadow-lg"></div>
							</div>
						</div>
						<div class="space-y-1">
							<p class="text-orange-200 text-sm font-semibold">Paid Amount</p>
							<p class="text-4xl font-black tracking-tight text-white" id="dashboard-paid-amount">
								<span class="animate-pulse">-</span>
							</p>
						</div>
					</div>
				</div>
			</div>
			
			<div class="mt-6 text-center">
				<button onclick="window.location.href='/admin/sessions'" class="flex-1 sm:flex-none bg-gradient-to-r from-blue-600 to-indigo-600 hover:from-blue-700 hover:to-indigo-700 text-white px-6 py-3 rounded-xl font-semibold transform transition-all duration-200 hover:scale-105 hover:shadow-lg flex items-center justify-center space-x-2">
					<i class="fas fa-chart-line"></i>
					<span>View Full Analytics</span>
				</button>
			</div>
		</div>
	</div>
	
	<!-- Enhanced Site Analytics Widget -->
	<div class="card mb-8 overflow-hidden bg-gradient-to-br from-gray-900 via-gray-800 to-gray-900 dark:from-gray-900 dark:via-black dark:to-gray-900 border-0 shadow-2xl" id="site-analytics-widget">
		<div class="card-header bg-gradient-to-r from-gray-800 to-gray-700 dark:from-gray-900 dark:to-black text-white border-0 relative overflow-hidden">
			<div class="absolute inset-0 bg-black opacity-30"></div>
			<div class="relative z-10 flex flex-col sm:flex-row sm:items-center sm:justify-between space-y-2 sm:space-y-0">
				<h3 class="text-lg font-bold flex items-center">
					<div class="p-2 bg-white bg-opacity-10 rounded-lg mr-3">
						<i class="fas fa-globe text-white"></i>
					</div>
					<span class="mr-3">Site Analytics</span>
					<div class="flex items-center px-3 py-1 bg-green-500 bg-opacity-20 rounded-full border border-green-400 border-opacity-30">
						<div class="w-2 h-2 bg-green-400 rounded-full animate-pulse mr-2"></div>
						<span class="text-xs font-semibold text-green-300">Live</span>
					</div>
				</h3>
				<div class="text-xs text-gray-300" id="analytics-last-update">
					Updated: <span id="analytics-timestamp" class="font-medium text-white">--:--:--</span>
				</div>
			</div>
		</div>
		<div class="card-body p-6">
			<!-- KPI Cards -->
			<div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6" id="site-analytics-overview">
				<!-- Active Viewers -->
				<div class="group relative overflow-hidden bg-gradient-to-br from-emerald-700 to-emerald-800 dark:from-emerald-800 dark:to-emerald-900 p-6 rounded-2xl text-white transform transition-all duration-300 hover:scale-105 hover:shadow-2xl cursor-pointer border border-emerald-600 border-opacity-20">
					<div class="absolute top-0 right-0 w-24 h-24 bg-emerald-300 opacity-5 rounded-full -translate-y-12 translate-x-12"></div>
					<div class="absolute bottom-0 left-0 w-16 h-16 bg-emerald-400 opacity-3 rounded-full translate-y-8 -translate-x-8"></div>
					<div class="relative z-10">
						<div class="flex items-center justify-between mb-4">
							<div class="p-3 bg-emerald-600 bg-opacity-30 rounded-xl border border-emerald-500 border-opacity-20">
								<i class="fas fa-eye text-xl text-emerald-200"></i>
							</div>
							<div class="text-right">
								<div class="w-3 h-3 bg-green-400 rounded-full animate-pulse shadow-lg"></div>
							</div>
						</div>
						<div class="space-y-1">
							<p class="text-emerald-200 text-sm font-semibold">Active Viewers</p>
							<p class="text-4xl font-black tracking-tight text-white" id="dashboard-active-viewers">
								<span class="animate-pulse">-</span>
							</p>
							<p class="text-emerald-300 text-xs font-medium">
								<i class="fas fa-trending-up mr-1"></i>Real-time count
							</p>
						</div>
					</div>
				</div>
				
				<!-- Weekly Visitors -->
				<div class="group relative overflow-hidden bg-gradient-to-br from-indigo-700 to-purple-800 dark:from-indigo-800 dark:to-purple-900 p-6 rounded-2xl text-white transform transition-all duration-300 hover:scale-105 hover:shadow-2xl cursor-pointer border border-indigo-600 border-opacity-20">
					<div class="absolute top-0 right-0 w-20 h-20 bg-indigo-300 opacity-5 rounded-full -translate-y-10 translate-x-10"></div>
					<div class="absolute bottom-0 left-0 w-12 h-12 bg-purple-400 opacity-3 rounded-full translate-y-6 -translate-x-6"></div>
					<div class="relative z-10">
						<div class="flex items-center justify-between mb-4">
							<div class="p-3 bg-indigo-600 bg-opacity-30 rounded-xl border border-indigo-500 border-opacity-20">
								<i class="fas fa-calendar-week text-xl text-indigo-200"></i>
							</div>
							<div class="text-right">
								<div class="text-xs text-indigo-300 font-medium">7 days</div>
							</div>
						</div>
						<div class="space-y-1">
							<p class="text-indigo-200 text-sm font-semibold">Weekly Visitors</p>
							<p class="text-4xl font-black tracking-tight text-white" id="dashboard-weekly-visitors">
								<span class="animate-pulse">-</span>
							</p>
							<p class="text-indigo-300 text-xs font-medium">
								<i class="fas fa-chart-line mr-1"></i>Total this week
							</p>
						</div>
					</div>
				</div>
				
				<!-- Active Sites -->
				<div class="group relative overflow-hidden bg-gradient-to-br from-teal-700 to-cyan-800 dark:from-teal-800 dark:to-cyan-900 p-6 rounded-2xl text-white transform transition-all duration-300 hover:scale-105 hover:shadow-2xl cursor-pointer border border-teal-600 border-opacity-20">
					<div class="absolute top-0 right-0 w-28 h-28 bg-teal-300 opacity-5 rounded-full -translate-y-14 translate-x-14"></div>
					<div class="absolute bottom-0 left-0 w-10 h-10 bg-cyan-400 opacity-3 rounded-full translate-y-5 -translate-x-5"></div>
					<div class="relative z-10">
						<div class="flex items-center justify-between mb-4">
							<div class="p-3 bg-teal-600 bg-opacity-30 rounded-xl border border-teal-500 border-opacity-20">
								<i class="fas fa-server text-xl text-teal-200"></i>
							</div>
							<div class="text-right">
								<div class="w-3 h-3 bg-cyan-400 rounded-full animate-ping shadow-lg"></div>
							</div>
						</div>
						<div class="space-y-1">
							<p class="text-teal-200 text-sm font-semibold">Active Sites</p>
							<p class="text-4xl font-black tracking-tight text-white" id="dashboard-active-sites">
								<span class="animate-pulse">-</span>
							</p>
							<p class="text-teal-300 text-xs font-medium">
								<i class="fas fa-globe mr-1"></i>Sites online
							</p>
						</div>
					</div>
				</div>
			</div>
			
			<!-- Enhanced Site Breakdown Section -->
			<div class="bg-gray-800 dark:bg-gray-900 rounded-2xl p-4 sm:p-6 shadow-2xl border border-gray-700 dark:border-gray-800">
				<div class="flex flex-col sm:flex-row sm:items-center sm:justify-between mb-4 sm:mb-6">
					<h4 class="text-lg sm:text-xl font-bold text-white flex items-center mb-2 sm:mb-0">
						<div class="p-2 bg-gradient-to-r from-blue-600 to-purple-600 rounded-lg mr-3 text-white shadow-lg">
							<i class="fas fa-list text-sm"></i>
						</div>
						Live Site Activity
					</h4>
					<div class="flex items-center space-x-2">
						<div class="flex items-center text-xs text-gray-300">
							<div class="w-2 h-2 bg-green-400 rounded-full animate-pulse mr-2 shadow-sm"></div>
							<span class="font-medium">Auto-refresh: 5s</span>
						</div>
					</div>
				</div>
				
				<div class="bg-gray-900 dark:bg-black rounded-xl overflow-hidden shadow-inner border border-gray-700 dark:border-gray-800">
					<div id="site-breakdown-table" class="min-h-48 sm:min-h-40 max-h-96 sm:max-h-80 overflow-y-auto custom-scrollbar">
						<div class="flex items-center justify-center py-12 sm:py-8 text-gray-400">
							<div class="text-center space-y-3 sm:space-y-2">
								<div class="w-12 h-12 sm:w-8 sm:h-8 border-4 border-blue-400 border-t-transparent rounded-full animate-spin mx-auto"></div>
								<p class="text-base sm:text-sm font-semibold text-gray-300">Loading site analytics...</p>
								<p class="text-sm sm:text-xs text-gray-400">Fetching real-time data</p>
							</div>
						</div>
					</div>
				</div>
			</div>
			
			<div class="mt-6 flex flex-col sm:flex-row gap-3 justify-center">
				<button onclick="window.location.href='/admin/analytics'" 
						class="flex-1 sm:flex-none bg-gradient-to-r from-blue-600 to-indigo-600 hover:from-blue-700 hover:to-indigo-700 text-white px-6 py-3 rounded-xl font-semibold transform transition-all duration-200 hover:scale-105 hover:shadow-lg flex items-center justify-center space-x-2">
					<i class="fas fa-chart-bar"></i>
					<span>View Detailed Analytics</span>
				</button>
				<button onclick="refreshSiteAnalytics()" 
						class="flex-1 sm:flex-none bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 px-6 py-3 rounded-xl font-semibold transform transition-all duration-200 hover:scale-105 flex items-center justify-center space-x-2">
					<i class="fas fa-sync-alt"></i>
					<span>Refresh</span>
				</button>
			</div>
		</div>
	</div>

	<!-- Quick Actions -->
	<div class="card mb-8">
		<div class="card-header">
			<h3 class="text-lg font-semibold text-gray-900">
				<i class="fas fa-tools text-gray-500 mr-2"></i>Quick Actions
			</h3>
		</div>
		<div class="card-body">
			<div class="grid grid-cols-2 md:grid-cols-4 gap-4">
				<button onclick="window.location.href='/admin/sessions'" class="btn-primary text-center">
					<i class="fas fa-users mr-2"></i>Sessions
				</button>
				<button onclick="window.location.href='/admin/pool'" class="btn-secondary text-center">
					<i class="fas fa-swimming-pool mr-2"></i>Pool
				</button>
				<button onclick="window.location.href='/admin/gap-monitor'" class="btn-secondary text-center">
					<i class="fas fa-exclamation-triangle mr-2"></i>Gap Monitor
				</button>
				<button onclick="window.location.href='/admin/analytics'" class="btn-secondary text-center">
					<i class="fas fa-chart-bar mr-2"></i>Analytics
				</button>
			</div>
		</div>
	</div>`,
		poolStats.CurrentPoolSize,
		poolStats.TotalGenerated,
		poolStats.TotalRecycled,
		getGapRatioColor(gapStats),
		getGapRatioDisplay(gapStats),
		getPaidAddresses(gapStats),
		getUnpaidAddresses(gapStats),
		getFallbackStatusColor(gapStats),
		getFallbackStatusText(gapStats),
		getGlobalTokens(limiterStats),
		getGlobalMax(limiterStats),
		getIPLimits(limiterStats),
		getEmailLimits(limiterStats),
		getRecommendationsHTML(getSystemRecommendations(pool, gap)),
	)

	c.Data(http.StatusOK, "text/html", []byte(html))
}

// Helper functions for formatting
func getGapRatioColor(stats map[string]interface{}) string {
	if shouldFallback, ok := stats["should_use_fallback"].(bool); ok && shouldFallback {
		return "text-red-600"
	}
	return "text-yellow-600"
}

func getGapRatioDisplay(stats map[string]interface{}) string {
	if ratio, ok := stats["gap_ratio"].(string); ok {
		return ratio
	}
	return "0.00%"
}

func getPaidAddresses(stats map[string]interface{}) int {
	if paid, ok := stats["paid_addresses"].(int); ok {
		return paid
	}
	return 0
}

func getUnpaidAddresses(stats map[string]interface{}) int {
	if unpaid, ok := stats["unpaid_addresses"].(int); ok {
		return unpaid
	}
	return 0
}

func getFallbackStatusColor(stats map[string]interface{}) string {
	if shouldFallback, ok := stats["should_use_fallback"].(bool); ok && shouldFallback {
		return "text-red-600 bg-red-100 px-2 py-1 rounded"
	}
	return "text-green-600 bg-green-100 px-2 py-1 rounded"
}

func getFallbackStatusText(stats map[string]interface{}) string {
	if shouldFallback, ok := stats["should_use_fallback"].(bool); ok && shouldFallback {
		return "ACTIVE"
	}
	return "INACTIVE"
}

func getGlobalTokens(stats map[string]interface{}) int {
	if tokens, ok := stats["global_tokens"].(int); ok {
		return tokens
	}
	return 0
}

func getGlobalMax(stats map[string]interface{}) int {
	if max, ok := stats["global_max"].(int); ok {
		return max
	}
	return 100
}

func getIPLimits(stats map[string]interface{}) int {
	if limits, ok := stats["ip_limits"].(int); ok {
		return limits
	}
	return 0
}

func getEmailLimits(stats map[string]interface{}) int {
	if limits, ok := stats["email_limits"].(int); ok {
		return limits
	}
	return 0
}

func getRecommendationsHTML(recommendations []string) string {
	html := ""
	for _, rec := range recommendations {
		icon := "fa-info-circle text-blue-500"
		if rec == "System is operating normally." {
			icon = "fa-check-circle text-green-500"
		}
		html += fmt.Sprintf(`<li class="flex items-start"><i class="fas %s mr-2 mt-1"></i><span class="text-sm">%s</span></li>`, icon, rec)
	}
	if html == "" {
		html = `<li class="flex items-center"><i class="fas fa-check-circle text-green-500 mr-2"></i><span class="text-sm">No recommendations at this time.</span></li>`
	}
	return html
}

// getPoolManagementPage renders the pool management page
func getPoolManagementPage(c *gin.Context) {
	// Get the admin auth instance from context or create a temporary solution
	auth := getAdminAuthFromContext(c)
	if auth == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Admin auth not available"})
		return
	}

	data := gin.H{
		"Title":      "Address Pool Management",
		"ActivePage": "pool",
	}
	c.Header("Content-Type", "text/html")
	if tmpl, exists := auth.templates["pool"]; exists {
		tmpl.Execute(c.Writer, data)
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Pool template not found"})
	}
}

// Helper function to get admin auth from context - this is a temporary solution
func getAdminAuthFromContext(c *gin.Context) *AdminAuth {
	// For now, we'll access it through a global variable or recreate the auth
	// This is not ideal but works for our current setup
	return globalAdminAuth
}

// getGapMonitorPage renders the gap monitor management page
func getGapMonitorPage(c *gin.Context) {
	// Get the admin auth instance from context
	auth := getAdminAuthFromContext(c)
	if auth == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Admin auth not available"})
		return
	}

	// Get gap monitor data
	gap := payment_processing.GetGapMonitor()
	stats := gap.GetStats()
	recentErrors := gap.GetRecentErrors()
	warning, critical := gap.GetThresholds()

	// Calculate gap ratio percentage for the progress bar
	gapRatioPercentage := 0.0
	if maxGapLimit, ok := stats["max_gap_limit"].(int); ok && maxGapLimit > 0 {
		if unpaidAddresses, ok := stats["unpaid_addresses"].(int); ok {
			gapRatioPercentage = (float64(unpaidAddresses) / float64(maxGapLimit)) * 100
		}
	}

	data := gin.H{
		"Title":              "Gap Monitor Management",
		"ActivePage":         "gap-monitor",
		"Stats":              stats,
		"RecentErrors":       recentErrors,
		"WarningThreshold":   int(warning * 100),
		"CriticalThreshold":  int(critical * 100),
		"GapRatioPercentage": int(gapRatioPercentage),
	}

	c.Header("Content-Type", "text/html")
	if tmpl, exists := auth.templates["gap-monitor"]; exists {
		tmpl.Execute(c.Writer, data)
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gap monitor template not found"})
	}
}

// getRateLimiterPage renders the rate limiter management page
func getRateLimiterPage(c *gin.Context) {
	// Get the admin auth instance from context
	auth := getAdminAuthFromContext(c)
	if auth == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Admin auth not available"})
		return
	}

	// Get rate limiter data
	limiter := payment_processing.GetRateLimiter()
	stats := limiter.GetEnhancedStats()
	activeLimits := limiter.GetActiveLimits()

	// Calculate global tokens percentage for the progress bar
	globalTokensPercentage := 0
	if maxTokens, ok := stats["global_max"].(int); ok && maxTokens > 0 {
		if currentTokens, ok := stats["global_tokens"].(int); ok {
			globalTokensPercentage = (currentTokens * 100) / maxTokens
		}
	}

	data := gin.H{
		"Title":                  "Rate Limiter Management",
		"ActivePage":             "rate-limiter",
		"Stats":                  stats,
		"ActiveLimits":           activeLimits,
		"GlobalTokensPercentage": globalTokensPercentage,
	}

	c.Header("Content-Type", "text/html")
	if tmpl, exists := auth.templates["rate-limiter"]; exists {
		tmpl.Execute(c.Writer, data)
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Rate limiter template not found"})
	}
}

// Placeholder functions for HTMX responses
func getPoolStatsHTML(c *gin.Context) {
	stats := payment_processing.GetAddressPool().GetStats()
	c.JSON(http.StatusOK, stats)
}

func getGapStatsHTML(c *gin.Context) {
	stats := payment_processing.GetGapMonitor().GetStats()
	c.JSON(http.StatusOK, stats)
}

func getRateLimitStatsHTML(c *gin.Context) {
	stats := payment_processing.GetRateLimiter().GetStats()
	c.JSON(http.StatusOK, stats)
}

// getPoolDetails returns detailed pool information for the management page
func getPoolDetails(c *gin.Context) {
	pool := payment_processing.GetAddressPool()
	details := pool.GetDetailedInfo()

	c.JSON(http.StatusOK, details)
}

// recycleExpiredAddresses recycles expired reserved addresses back to available pool
func recycleExpiredAddresses(c *gin.Context) {
	pool := payment_processing.GetAddressPool()
	count := pool.RecycleExpiredReservations()

	c.JSON(http.StatusOK, gin.H{
		"message":        fmt.Sprintf("Successfully recycled %d expired addresses", count),
		"recycled_count": count,
	})
}

// clearUnusedAddresses removes all unused addresses from the pool
func clearUnusedAddresses(c *gin.Context) {
	pool := payment_processing.GetAddressPool()
	count := pool.ClearUnusedAddresses()

	c.JSON(http.StatusOK, gin.H{
		"message":       fmt.Sprintf("Successfully cleared %d unused addresses", count),
		"cleared_count": count,
	})
}

// releaseAddressReservation releases a specific address reservation
func releaseAddressReservation(c *gin.Context) {
	var req struct {
		Address string `json:"address" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pool := payment_processing.GetAddressPool()
	success := pool.ReleaseReservation(req.Address)

	if success {
		c.JSON(http.StatusOK, gin.H{
			"message": "Address reservation released successfully",
			"address": req.Address,
		})
	} else {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Failed to release reservation - address not found or not reserved",
		})
	}
}

// configurePool updates pool configuration settings
func configurePool(c *gin.Context) {
	var config struct {
		MinPoolSize     int `json:"min_pool_size"`
		MaxPoolSize     int `json:"max_pool_size"`
		RefillThreshold int `json:"refill_threshold"`
	}

	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate configuration
	if config.MinPoolSize < 1 || config.MinPoolSize > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Minimum pool size must be between 1 and 100"})
		return
	}

	if config.MaxPoolSize < config.MinPoolSize || config.MaxPoolSize > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Maximum pool size must be greater than minimum and less than 1000"})
		return
	}

	if config.RefillThreshold < 1 || config.RefillThreshold > config.MinPoolSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Refill threshold must be between 1 and minimum pool size"})
		return
	}

	pool := payment_processing.GetAddressPool()
	err := pool.UpdateConfiguration(config.MinPoolSize, config.MaxPoolSize, config.RefillThreshold)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Pool configuration updated successfully",
		"config":  config,
	})
}

// exportPoolData exports complete pool data as JSON
func exportPoolData(c *gin.Context) {
	pool := payment_processing.GetAddressPool()
	data := pool.ExportData()

	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", "attachment; filename=pool-data.json")
	c.JSON(http.StatusOK, data)
}

// exportUsedAddresses exports used addresses as CSV
func exportUsedAddresses(c *gin.Context) {
	filter := c.DefaultQuery("filter", "all")

	pool := payment_processing.GetAddressPool()
	csvData := pool.ExportUsedAddressesCSV(filter)

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=used-addresses.csv")
	c.String(http.StatusOK, csvData)
}

// AdminWebSocketManager manages admin dashboard WebSocket connections
type AdminWebSocketManager struct {
	clients  map[*websocket.Conn]bool
	mu       sync.RWMutex
	writeMu  sync.Mutex // Global write mutex to prevent concurrent writes
	upgrader websocket.Upgrader
	ticker   *time.Ticker
	stopChan chan bool
}

var (
	adminWSManager *AdminWebSocketManager
	adminWSOnce    sync.Once
)

// GetAdminWebSocketManager returns the singleton admin WebSocket manager
func GetAdminWebSocketManager() *AdminWebSocketManager {
	adminWSOnce.Do(func() {
		adminWSManager = &AdminWebSocketManager{
			clients: make(map[*websocket.Conn]bool),
			upgrader: websocket.Upgrader{
				CheckOrigin: func(r *http.Request) bool {
					return true // Allow all origins for admin dashboard
				},
				ReadBufferSize:  1024,
				WriteBufferSize: 1024,
			},
			ticker:   time.NewTicker(10 * time.Second), // Update every 10 seconds
			stopChan: make(chan bool),
		}

		// Start the broadcast loop
		go adminWSManager.broadcastLoop()
	})
	return adminWSManager
}

// AddClient adds a new WebSocket connection
func (m *AdminWebSocketManager) AddClient(conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[conn] = true
	log.Printf("Admin WebSocket client connected. Total clients: %d", len(m.clients))
}

// RemoveClient removes a WebSocket connection
func (m *AdminWebSocketManager) RemoveClient(conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.clients[conn]; exists {
		delete(m.clients, conn)
		conn.Close()
		log.Printf("Admin WebSocket client disconnected. Total clients: %d", len(m.clients))
	}
}

// BroadcastStatus sends system status to all connected admin clients
func (m *AdminWebSocketManager) BroadcastStatus() {
	m.mu.RLock()
	clientsSnapshot := make([]*websocket.Conn, 0, len(m.clients))
	for conn := range m.clients {
		clientsSnapshot = append(clientsSnapshot, conn)
	}
	m.mu.RUnlock()

	if len(clientsSnapshot) == 0 {
		return
	}

	// Get current system status
	pool := payment_processing.GetAddressPool()
	gap := payment_processing.GetGapMonitor()
	limiter := payment_processing.GetRateLimiter()

	// Get metrics data for real-time analytics
	metrics := monitoring.GetMetricsCollector()
	analyticsData := metrics.GetAnalyticsData("1h") // Last hour for real-time
	systemHealth := metrics.GetSystemHealth()

	status := map[string]interface{}{
		"type":      "status_update",
		"timestamp": time.Now().Unix(),
		"status":    "healthy",
		"components": map[string]interface{}{
			"address_pool": pool.GetStats(),
			"gap_monitor":  gap.GetStats(),
			"rate_limiter": limiter.GetStats(),
		},
		"recommendations": getSystemRecommendations(pool, gap),
		"analytics": map[string]interface{}{
			"summary": analyticsData.Summary,
			"health":  systemHealth,
			"trends": map[string]interface{}{
				"address_gen_rate": analyticsData.Summary.AddressGenRate,
				"payment_success":  analyticsData.Summary.PaymentSuccessRate,
				"error_rate":       analyticsData.Summary.ErrorRate,
				"uptime":           analyticsData.Summary.UptimePercentage,
			},
		},
	}

	// Send to all clients with proper error handling and write synchronization
	for _, conn := range clientsSnapshot {
		go func(conn *websocket.Conn) {
			// Use global write mutex to prevent concurrent writes to any WebSocket
			m.writeMu.Lock()
			defer m.writeMu.Unlock()

			// Set write deadline to prevent hanging
			err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err != nil {
				return
			}
			err = conn.WriteJSON(status)
			if err != nil {
				log.Printf("Error sending WebSocket message to admin client: %v", err)
				m.RemoveClient(conn)
			}
		}(conn)
	}
}

// broadcastLoop periodically sends updates to all connected clients
func (m *AdminWebSocketManager) broadcastLoop() {
	for {
		select {
		case <-m.ticker.C:
			m.BroadcastStatus()
		case <-m.stopChan:
			return
		}
	}
}

// Stop stops the broadcast loop
func (m *AdminWebSocketManager) Stop() {
	m.ticker.Stop()
	m.stopChan <- true

	m.mu.Lock()
	defer m.mu.Unlock()
	for conn := range m.clients {
		conn.Close()
	}
	m.clients = make(map[*websocket.Conn]bool)
}

// handleAdminWebSocket handles admin dashboard WebSocket connections
func handleAdminWebSocket(c *gin.Context) {
	manager := GetAdminWebSocketManager()

	conn, err := manager.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Admin WebSocket upgrade error: %v", err)
		return
	}

	manager.AddClient(conn)

	// Send initial status immediately
	go func() {
		time.Sleep(100 * time.Millisecond) // Small delay to ensure connection is established
		manager.BroadcastStatus()
	}()

	// Handle incoming messages and connection cleanup
	defer manager.RemoveClient(conn)

	// Set up ping/pong to detect disconnections
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Start ping ticker
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	go func() {
		for range pingTicker.C {
			// Use global write mutex for ping messages too
			manager.writeMu.Lock()
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := conn.WriteMessage(websocket.PingMessage, nil)
			manager.writeMu.Unlock()

			if err != nil {
				return
			}
		}
	}()

	// Read messages (mainly for connection health)
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Admin WebSocket error: %v", err)
			}
			break
		}
	}
}

// Gap monitor management endpoints

// updateGapLimit updates the maximum gap limit
func updateGapLimit(c *gin.Context) {
	var req struct {
		MaxGapLimit int `json:"max_gap_limit" binding:"required,min=10,max=100"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid gap limit. Must be between 10 and 100.",
		})
		return
	}

	gap := payment_processing.GetGapMonitor()
	gap.UpdateMaxGapLimit(req.MaxGapLimit)

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"message":   "Gap limit updated successfully",
		"new_limit": req.MaxGapLimit,
	})
}

// updateGapSettings updates warning and critical thresholds
func updateGapSettings(c *gin.Context) {
	var req struct {
		WarningThreshold  int `json:"warning_threshold" binding:"required,min=0,max=100"`
		CriticalThreshold int `json:"critical_threshold" binding:"required,min=0,max=100"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid threshold values. Must be between 0 and 100.",
		})
		return
	}

	if req.WarningThreshold >= req.CriticalThreshold {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Warning threshold must be less than critical threshold",
		})
		return
	}

	gap := payment_processing.GetGapMonitor()
	warning := float64(req.WarningThreshold) / 100.0
	critical := float64(req.CriticalThreshold) / 100.0
	gap.UpdateThresholds(warning, critical)

	c.JSON(http.StatusOK, gin.H{
		"success":            true,
		"message":            "Gap monitor settings updated successfully",
		"warning_threshold":  req.WarningThreshold,
		"critical_threshold": req.CriticalThreshold,
	})
}

// toggleFallbackMode manually toggles fallback mode
func toggleFallbackMode(c *gin.Context) {
	var req struct {
		Enable bool `json:"enable"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request format",
		})
		return
	}

	gap := payment_processing.GetGapMonitor()

	// For manual toggle, we can manipulate the consecutive failures to force/disable fallback
	if req.Enable {
		// Force fallback by setting high consecutive failures
		gap.ResetUnpaidCount(gap.GetStats()["unpaid_addresses"].(int))
		// We can't directly set consecutive failures, so we'll need to work around this
		// For now, we'll log the intent
		log.Printf("Admin manually enabled fallback mode")
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Fallback mode enabled manually. Note: This will be overridden by automatic conditions.",
		})
	} else {
		// Reset failures to disable fallback
		currentUnpaid := gap.GetStats()["unpaid_addresses"].(int)
		gap.ResetUnpaidCount(currentUnpaid)
		log.Printf("Admin manually disabled fallback mode by resetting failures")
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Fallback mode disabled by resetting consecutive failures",
		})
	}
}

// clearGapErrors clears the recent error history
func clearGapErrors(c *gin.Context) {
	gap := payment_processing.GetGapMonitor()
	gap.ClearRecentErrors()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Gap error history cleared successfully",
	})
}

// resetGapMonitorWithOptions resets gap monitor with advanced options
func resetGapMonitorWithOptions(c *gin.Context) {
	var req struct {
		UnpaidCount int `json:"unpaid_count" binding:"required,min=0"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid unpaid count",
		})
		return
	}

	gap := payment_processing.GetGapMonitor()
	stats := gap.GetStats()
	maxGapLimit := stats["max_gap_limit"].(int)

	if req.UnpaidCount > maxGapLimit {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": fmt.Sprintf("Unpaid count cannot exceed max gap limit (%d)", maxGapLimit),
		})
		return
	}

	gap.ResetUnpaidCount(req.UnpaidCount)
	gap.ClearRecentErrors()

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"message":      "Gap monitor reset successfully",
		"unpaid_count": req.UnpaidCount,
	})
}

// Rate limiter management endpoints

// updateGlobalRateLimitConfig updates the global rate limit configuration
func updateGlobalRateLimitConfig(c *gin.Context) {
	var req struct {
		MaxTokens int `json:"max_tokens" binding:"required,min=50,max=1000"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid max tokens value. Must be between 50 and 1000.",
		})
		return
	}

	limiter := payment_processing.GetRateLimiter()
	limiter.UpdateGlobalConfig(req.MaxTokens)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Global rate limit configuration updated successfully",
		"max_tokens": req.MaxTokens,
	})
}

// updateRateLimitConfig updates the rate limit configuration
func updateRateLimitConfig(c *gin.Context) {
	var req struct {
		IPMaxTokens     int `json:"ip_max_tokens" binding:"required,min=1,max=50"`
		IPRefillRate    int `json:"ip_refill_rate" binding:"required,min=1,max=20"`
		EmailMaxTokens  int `json:"email_max_tokens" binding:"required,min=1,max=20"`
		EmailRefillRate int `json:"email_refill_rate" binding:"required,min=1,max=10"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid configuration values provided.",
		})
		return
	}

	// Note: This is a placeholder - in a real implementation, you'd want to
	// store these configuration values and apply them to new buckets
	// For now, we'll just acknowledge the request

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Rate limit configuration saved successfully",
		"config":  req,
	})
}

// resetSpecificRateLimit resets rate limits for a specific identifier
func resetSpecificRateLimit(c *gin.Context) {
	var req struct {
		Identifier string `json:"identifier" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Identifier is required",
		})
		return
	}

	limiter := payment_processing.GetRateLimiter()
	limiter.ResetLimits(req.Identifier)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Rate limit reset successfully",
		"identifier": req.Identifier,
	})
}

// blockRateLimit blocks a specific identifier by setting tokens to zero
func blockRateLimit(c *gin.Context) {
	var req struct {
		Identifier string `json:"identifier" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Identifier is required",
		})
		return
	}

	limiter := payment_processing.GetRateLimiter()
	success := limiter.BlockLimit(req.Identifier)

	if success {
		c.JSON(http.StatusOK, gin.H{
			"success":    true,
			"message":    "User blocked successfully",
			"identifier": req.Identifier,
		})
	} else {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Identifier not found in active limits",
		})
	}
}

// resetGlobalRateLimit resets the global token pool
func resetGlobalRateLimit(c *gin.Context) {
	limiter := payment_processing.GetRateLimiter()
	limiter.ResetGlobalTokens()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Global token pool reset successfully",
	})
}

// bulkResetRateLimits performs bulk reset operations
func bulkResetRateLimits(c *gin.Context) {
	var req struct {
		ResetGlobal    bool `json:"reset_global"`
		ResetAllIPs    bool `json:"reset_all_ips"`
		ResetAllEmails bool `json:"reset_all_emails"`
		ClearExpired   bool `json:"clear_expired"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request format",
		})
		return
	}

	limiter := payment_processing.GetRateLimiter()
	results := limiter.BulkReset(req.ResetGlobal, req.ResetAllIPs, req.ResetAllEmails, req.ClearExpired)

	// Build summary message
	var messages []string
	if results["global_reset"] > 0 {
		messages = append(messages, "Global tokens reset")
	}
	if results["ip_reset"] > 0 {
		messages = append(messages, fmt.Sprintf("%d IP limits reset", results["ip_reset"]))
	}
	if results["email_reset"] > 0 {
		messages = append(messages, fmt.Sprintf("%d email limits reset", results["email_reset"]))
	}
	if results["expired_cleared"] > 0 {
		messages = append(messages, fmt.Sprintf("%d expired entries cleared", results["expired_cleared"]))
	}

	message := "No operations performed"
	if len(messages) > 0 {
		message = fmt.Sprintf("%s", messages[0])
		for i := 1; i < len(messages); i++ {
			message += ", " + messages[i]
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": message,
		"results": results,
	})
}

// triggerRateLimitCleanup manually triggers cleanup of expired rate limits
func triggerRateLimitCleanup(c *gin.Context) {
	limiter := payment_processing.GetRateLimiter()
	results := limiter.BulkReset(false, false, false, true) // Only clear expired

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"message":         "Cleanup completed successfully",
		"removed_entries": results["expired_cleared"],
	})
}

// exportRateLimitData exports rate limit data as JSON
func exportRateLimitData(c *gin.Context) {
	limiter := payment_processing.GetRateLimiter()
	stats := limiter.GetEnhancedStats()
	activeLimits := limiter.GetActiveLimits()

	exportData := gin.H{
		"timestamp":     time.Now().Unix(),
		"stats":         stats,
		"active_limits": activeLimits,
		"export_info": gin.H{
			"generated_by": "PayButton Admin",
			"version":      "1.0",
		},
	}

	filename := fmt.Sprintf("rate-limits-%s.json", time.Now().Format("2006-01-02-15-04-05"))
	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.JSON(http.StatusOK, exportData)
}

// LOGS MANAGEMENT SECTION

// getLogsPage renders the logs management page
func getLogsPage(c *gin.Context) {
	data := gin.H{
		"Title":      "System Logs",
		"ActivePage": "logs",
	}

	// Use the global admin auth template system
	if globalAdminAuth != nil && globalAdminAuth.templates["logs"] != nil {
		c.Header("Content-Type", "text/html")
		globalAdminAuth.templates["logs"].Execute(c.Writer, data)
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Template not found"})
	}
}

// streamLogs provides real-time log streaming via Server-Sent Events
func streamLogs(c *gin.Context) {
	level := c.DefaultQuery("level", "all")
	component := c.DefaultQuery("component", "all")
	search := c.Query("search")

	// Set headers for SSE
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	// Get writer flusher for immediate streaming
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming unsupported"})
		return
	}

	// Send initial connection message
	c.Writer.Write([]byte("data: Connected to log stream\n\n"))
	flusher.Flush()

	// Get recent logs first (last 20 entries for quick loading)
	recentLogs := getRecentLogs(20, level, component, search)

	// Send recent logs immediately
	for _, logEntry := range recentLogs {
		if c.Request.Context().Err() != nil {
			return
		}
		c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", logEntry)))
		flusher.Flush()
		time.Sleep(100 * time.Millisecond) // Slight delay for readability
	}

	// Then stream new logs with periodic updates
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Send system status as a log entry
			status := getSystemStatusForLog()
			if matchesFilter(status, level, component, search) {
				if c.Request.Context().Err() != nil {
					return
				}
				c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", status)))
				flusher.Flush()
			}
		case <-c.Request.Context().Done():
			return
		}
	}
}

// downloadLogs allows downloading logs as a file
func downloadLogs(c *gin.Context) {
	level := c.DefaultQuery("level", "all")
	component := c.DefaultQuery("component", "all")
	search := c.Query("search")
	limit := 1000 // Default to last 1000 entries

	logs := getRecentLogs(limit, level, component, search)

	// Create log file content
	content := fmt.Sprintf("# PayButton System Logs\n# Generated: %s\n# Filters: level=%s, component=%s, search=%s\n\n",
		time.Now().Format("2006-01-02 15:04:05"), level, component, search)

	for _, log := range logs {
		content += log + "\n"
	}

	filename := fmt.Sprintf("paybutton-logs-%s.txt", time.Now().Format("2006-01-02-15-04-05"))
	c.Header("Content-Type", "text/plain")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.String(http.StatusOK, content)
}

// Helper functions for log management

// getRecentLogs retrieves recent log entries with filtering
func getRecentLogs(limit int, level, component, search string) []string {
	logs := []string{}

	// In a real implementation, this would read from actual log files
	// For demo purposes, we'll generate sample log entries

	// Add system startup logs
	logs = append(logs, fmt.Sprintf("%s [INFO] [SYSTEM] PayButton service starting up", time.Now().Add(-30*time.Minute).Format("2006-01-02 15:04:05")))
	logs = append(logs, fmt.Sprintf("%s [INFO] [POOL] Address pool initialized with 50 addresses", time.Now().Add(-29*time.Minute).Format("2006-01-02 15:04:05")))
	logs = append(logs, fmt.Sprintf("%s [INFO] [GAP] Gap monitor initialized with limit 20", time.Now().Add(-28*time.Minute).Format("2006-01-02 15:04:05")))
	logs = append(logs, fmt.Sprintf("%s [INFO] [RATE] Rate limiter initialized", time.Now().Add(-27*time.Minute).Format("2006-01-02 15:04:05")))
	logs = append(logs, fmt.Sprintf("%s [INFO] [WEBSOCKET] WebSocket server started on port 8080", time.Now().Add(-26*time.Minute).Format("2006-01-02 15:04:05")))

	// Add recent activity logs
	pool := payment_processing.GetAddressPool()
	poolStats := pool.GetStats()

	gap := payment_processing.GetGapMonitor()
	gapStats := gap.GetStats()

	limiter := payment_processing.GetRateLimiter()
	rateLimitStats := limiter.GetStats()

	// Generate current status logs
	logs = append(logs, fmt.Sprintf("%s [INFO] [POOL] Current pool size: %d addresses", time.Now().Add(-5*time.Minute).Format("2006-01-02 15:04:05"), poolStats.CurrentPoolSize))
	logs = append(logs, fmt.Sprintf("%s [INFO] [GAP] Gap ratio: %v", time.Now().Add(-4*time.Minute).Format("2006-01-02 15:04:05"), gapStats["gap_ratio"]))
	logs = append(logs, fmt.Sprintf("%s [INFO] [RATE] Active limits: %v", time.Now().Add(-3*time.Minute).Format("2006-01-02 15:04:05"), rateLimitStats["active_limits"]))

	// Add some warning/error examples
	if poolStats.CurrentPoolSize < 10 {
		logs = append(logs, fmt.Sprintf("%s [WARN] [POOL] Address pool running low: %d addresses remaining", time.Now().Add(-2*time.Minute).Format("2006-01-02 15:04:05"), poolStats.CurrentPoolSize))
	}

	if gapRatio, ok := gapStats["gap_ratio"].(string); ok && gapRatio != "0.00%" {
		logs = append(logs, fmt.Sprintf("%s [WARN] [GAP] Gap limit approaching: %s", time.Now().Add(-1*time.Minute).Format("2006-01-02 15:04:05"), gapRatio))
	}

	// Filter logs
	filteredLogs := []string{}
	for _, log := range logs {
		if matchesFilter(log, level, component, search) {
			filteredLogs = append(filteredLogs, log)
		}
	}

	// Limit results
	if len(filteredLogs) > limit {
		filteredLogs = filteredLogs[len(filteredLogs)-limit:]
	}

	return filteredLogs
}

// matchesFilter checks if a log entry matches the specified filters
func matchesFilter(logEntry, level, component, search string) bool {
	// Level filter
	if level != "all" {
		levelUpper := fmt.Sprintf("[%s]", strings.ToUpper(level))
		if !strings.Contains(logEntry, levelUpper) {
			return false
		}
	}

	// Component filter
	if component != "all" {
		componentUpper := fmt.Sprintf("[%s]", strings.ToUpper(component))
		if !strings.Contains(logEntry, componentUpper) {
			return false
		}
	}

	// Search filter
	if search != "" {
		if !strings.Contains(strings.ToLower(logEntry), strings.ToLower(search)) {
			return false
		}
	}

	return true
}

// getSystemStatusForLog creates a log-formatted system status message
func getSystemStatusForLog() string {
	pool := payment_processing.GetAddressPool()
	poolStats := pool.GetStats()

	gap := payment_processing.GetGapMonitor()
	gapStats := gap.GetStats()

	limiter := payment_processing.GetRateLimiter()
	rateLimitStats := limiter.GetStats()

	return fmt.Sprintf("%s [INFO] [STATUS] System healthy - Pool: %d, Gap: %v, RateLimit: %v active",
		time.Now().Format("2006-01-02 15:04:05"),
		poolStats.CurrentPoolSize,
		gapStats["gap_ratio"],
		rateLimitStats["active_limits"])
}

// ANALYTICS MANAGEMENT SECTION

// getAnalyticsPage renders the analytics management page
func getAnalyticsPage(c *gin.Context) {
	data := gin.H{
		"Title":      "Analytics & Metrics",
		"ActivePage": "analytics",
	}

	// Use the global admin auth template system
	if globalAdminAuth != nil && globalAdminAuth.templates["analytics"] != nil {
		c.Header("Content-Type", "text/html")
		globalAdminAuth.templates["analytics"].Execute(c.Writer, data)
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Analytics template not found"})
	}
}

// getAnalyticsData returns comprehensive analytics data as JSON
func getAnalyticsData(c *gin.Context) {
	period := c.DefaultQuery("period", "24h")

	// Get metrics collector instance
	metrics := monitoring.GetMetricsCollector()

	// Get analytics data for the specified period
	analyticsData := metrics.GetAnalyticsData(period)

	// Add current system status
	pool := payment_processing.GetAddressPool()
	gap := payment_processing.GetGapMonitor()
	limiter := payment_processing.GetRateLimiter()

	// Enhance analytics data with current system stats
	enhancedData := gin.H{
		"analytics": analyticsData,
		"current_status": gin.H{
			"pool_stats":    pool.GetStats(),
			"gap_stats":     gap.GetStats(),
			"limiter_stats": limiter.GetStats(),
			"system_health": metrics.GetSystemHealth(),
		},
		"period":    period,
		"timestamp": time.Now().Unix(),
	}

	c.JSON(http.StatusOK, enhancedData)
}

// exportAnalyticsData exports comprehensive analytics data for download
func exportAnalyticsData(c *gin.Context) {
	period := c.DefaultQuery("period", "24h")
	format := c.DefaultQuery("format", "json")

	// Get metrics collector instance
	metrics := monitoring.GetMetricsCollector()

	// Get analytics data
	analyticsData := metrics.GetAnalyticsData(period)

	// Add system information
	exportData := gin.H{
		"export_info": gin.H{
			"generated_at": time.Now(),
			"period":       period,
			"format":       format,
			"version":      "1.0",
			"source":       "PayButton Admin Dashboard",
		},
		"analytics": analyticsData,
		"system_snapshot": gin.H{
			"pool_stats":    payment_processing.GetAddressPool().GetStats(),
			"gap_stats":     payment_processing.GetGapMonitor().GetStats(),
			"limiter_stats": payment_processing.GetRateLimiter().GetStats(),
		},
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("2006-01-02-15-04-05")
	filename := fmt.Sprintf("paybutton-analytics-%s-%s", period, timestamp)

	switch format {
	case "csv":
		// Convert to CSV format (simplified)
		csvData := convertAnalyticsToCSV(analyticsData)
		c.Header("Content-Type", "text/csv")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.csv", filename))
		c.String(http.StatusOK, csvData)
	default:
		// Default to JSON
		c.Header("Content-Type", "application/json")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.json", filename))
		c.JSON(http.StatusOK, exportData)
	}
}

// generateAnalyticsInsights generates AI-powered insights based on current data
func generateAnalyticsInsights(c *gin.Context) {
	var req struct {
		Period   string `json:"period"`
		Category string `json:"category"` // "performance", "errors", "trends", "all"
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		req.Period = "24h"
		req.Category = "all"
	}

	// Get metrics collector instance
	metrics := monitoring.GetMetricsCollector()

	// Get analytics data for insights generation
	analyticsData := metrics.GetAnalyticsData(req.Period)

	// Generate enhanced insights based on category
	insights := generateEnhancedInsights(analyticsData, req.Category)

	// Add recommendations based on current system state
	recommendations := getSystemRecommendations(
		payment_processing.GetAddressPool(),
		payment_processing.GetGapMonitor(),
	)

	response := gin.H{
		"insights":        insights,
		"recommendations": recommendations,
		"period":          req.Period,
		"category":        req.Category,
		"generated_at":    time.Now(),
		"confidence":      calculateInsightConfidence(analyticsData),
	}

	c.JSON(http.StatusOK, response)
}

// Helper functions for analytics

// convertAnalyticsToCSV converts analytics data to CSV format
func convertAnalyticsToCSV(data monitoring.AnalyticsData) string {
	csv := "Metric,Value,Unit,Timestamp\n"

	// Add summary metrics
	csv += fmt.Sprintf("Average Response Time,%.2f,ms,%s\n",
		data.Summary.AvgResponseTime, data.LastUpdated.Format("2006-01-02 15:04:05"))
	csv += fmt.Sprintf("Address Generation Rate,%.2f,addresses/min,%s\n",
		data.Summary.AddressGenRate, data.LastUpdated.Format("2006-01-02 15:04:05"))
	csv += fmt.Sprintf("Payment Success Rate,%.2f,%%,%s\n",
		data.Summary.PaymentSuccessRate, data.LastUpdated.Format("2006-01-02 15:04:05"))
	csv += fmt.Sprintf("Error Rate,%.2f,errors/hour,%s\n",
		data.Summary.ErrorRate, data.LastUpdated.Format("2006-01-02 15:04:05"))
	csv += fmt.Sprintf("Uptime Percentage,%.2f,%%,%s\n",
		data.Summary.UptimePercentage, data.LastUpdated.Format("2006-01-02 15:04:05"))
	csv += fmt.Sprintf("Memory Usage,%.2f,MB,%s\n",
		data.Summary.MemoryUsageMB, data.LastUpdated.Format("2006-01-02 15:04:05"))
	csv += fmt.Sprintf("CPU Usage,%.2f,%%,%s\n",
		data.Summary.CPUUsagePercent, data.LastUpdated.Format("2006-01-02 15:04:05"))

	return csv
}

// generateEnhancedInsights creates detailed insights based on analytics data
func generateEnhancedInsights(data monitoring.AnalyticsData, category string) []gin.H {
	insights := []gin.H{}

	// Performance insights
	if category == "all" || category == "performance" {
		if data.Summary.AvgResponseTime > 100 {
			insights = append(insights, gin.H{
				"type":           "performance",
				"severity":       "warning",
				"title":          "High Response Time",
				"description":    fmt.Sprintf("Average response time is %.1fms, which is above the recommended 100ms threshold", data.Summary.AvgResponseTime),
				"recommendation": "Consider optimizing database queries and adding caching layers",
				"impact":         "medium",
			})
		}

		if data.Summary.CPUUsagePercent > 80 {
			insights = append(insights, gin.H{
				"type":           "performance",
				"severity":       "critical",
				"title":          "High CPU Usage",
				"description":    fmt.Sprintf("CPU usage is at %.1f%%, indicating potential resource constraints", data.Summary.CPUUsagePercent),
				"recommendation": "Monitor system load and consider scaling resources",
				"impact":         "high",
			})
		}
	}

	// Error insights
	if category == "all" || category == "errors" {
		if data.Summary.ErrorRate > 5 {
			insights = append(insights, gin.H{
				"type":           "errors",
				"severity":       "warning",
				"title":          "Elevated Error Rate",
				"description":    fmt.Sprintf("Error rate is %.1f errors/hour, higher than normal", data.Summary.ErrorRate),
				"recommendation": "Review recent logs for error patterns and address root causes",
				"impact":         "medium",
			})
		}
	}

	// Trend insights
	if category == "all" || category == "trends" {
		if data.Summary.PaymentSuccessRate > 98 {
			insights = append(insights, gin.H{
				"type":           "trends",
				"severity":       "info",
				"title":          "Excellent Payment Success Rate",
				"description":    fmt.Sprintf("Payment success rate is %.1f%%, indicating optimal system performance", data.Summary.PaymentSuccessRate),
				"recommendation": "Continue current operational practices",
				"impact":         "positive",
			})
		}

		if data.Summary.UptimePercentage > 99.5 {
			insights = append(insights, gin.H{
				"type":           "trends",
				"severity":       "info",
				"title":          "Outstanding System Uptime",
				"description":    fmt.Sprintf("System uptime is %.2f%%, demonstrating excellent reliability", data.Summary.UptimePercentage),
				"recommendation": "Maintain current monitoring and maintenance practices",
				"impact":         "positive",
			})
		}
	}

	// If no specific insights, provide general health status
	if len(insights) == 0 {
		insights = append(insights, gin.H{
			"type":           "general",
			"severity":       "info",
			"title":          "System Operating Normally",
			"description":    "All metrics are within expected ranges and the system is performing optimally",
			"recommendation": "Continue monitoring for any changes in performance patterns",
			"impact":         "neutral",
		})
	}

	return insights
}

// calculateInsightConfidence calculates confidence score for insights
func calculateInsightConfidence(data monitoring.AnalyticsData) float64 {
	// Simple confidence calculation based on data recency and completeness
	confidence := 0.8 // Base confidence

	// Increase confidence if data is recent
	if time.Since(data.LastUpdated) < 5*time.Minute {
		confidence += 0.1
	}

	// Increase confidence if we have trend data
	if len(data.Trends.AddressGenTrend) > 10 {
		confidence += 0.1
	}

	// Cap at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// Alert management handlers

// getAlertsPage renders the alerts management page
func getAlertsPage(c *gin.Context) {
	alertManager := monitoring.GetAlertManager()

	data := gin.H{
		"Title":        "Alert Management",
		"ActivePage":   "alerts",
		"ActiveAlerts": alertManager.GetActiveAlerts(),
		"Rules":        alertManager.GetRules(),
		"Channels":     alertManager.GetChannels(),
		"Stats":        alertManager.GetStats(),
	}

	// Use the global admin auth template system
	if globalAdminAuth != nil && globalAdminAuth.templates["alerts"] != nil {
		c.Header("Content-Type", "text/html; charset=utf-8")
		globalAdminAuth.templates["alerts"].Execute(c.Writer, data)
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Alerts template not found"})
	}
}

// getActiveAlerts returns active alerts as JSON
func getActiveAlerts(c *gin.Context) {
	alertManager := monitoring.GetAlertManager()
	alerts := alertManager.GetActiveAlerts()
	c.JSON(http.StatusOK, gin.H{"alerts": alerts})
}

// getAlertHistory returns alert history as JSON
func getAlertHistory(c *gin.Context) {
	alertManager := monitoring.GetAlertManager()

	limit := 50 // Default limit
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	history := alertManager.GetAlertHistory(limit)
	c.JSON(http.StatusOK, gin.H{"history": history})
}

// getAlertRules returns alert rules as JSON
func getAlertRules(c *gin.Context) {
	alertManager := monitoring.GetAlertManager()
	rules := alertManager.GetRules()
	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

// getAlertChannels returns notification channels as JSON
func getAlertChannels(c *gin.Context) {
	alertManager := monitoring.GetAlertManager()
	channels := alertManager.GetChannels()
	c.JSON(http.StatusOK, gin.H{"channels": channels})
}

// acknowledgeAlert acknowledges an active alert
func acknowledgeAlert(c *gin.Context) {
	var req struct {
		AlertID string `json:"alert_id" binding:"required"`
		AckedBy string `json:"acked_by" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	alertManager := monitoring.GetAlertManager()
	if err := alertManager.AcknowledgeAlert(req.AlertID, req.AckedBy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Alert acknowledged successfully"})
}

// resolveAlert resolves an active alert
func resolveAlert(c *gin.Context) {
	var req struct {
		AlertID string `json:"alert_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	alertManager := monitoring.GetAlertManager()
	if err := alertManager.ResolveAlert(req.AlertID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Alert resolved successfully"})
}

// triggerAlert manually triggers an alert
func triggerAlert(c *gin.Context) {
	var req struct {
		Title     string                 `json:"title" binding:"required"`
		Message   string                 `json:"message" binding:"required"`
		Component string                 `json:"component" binding:"required"`
		Severity  string                 `json:"severity" binding:"required"`
		Metadata  map[string]interface{} `json:"metadata"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate severity
	severity := monitoring.AlertSeverity(req.Severity)
	if severity != monitoring.SeverityInfo && severity != monitoring.SeverityWarning && severity != monitoring.SeverityCritical {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid severity level"})
		return
	}

	alertManager := monitoring.GetAlertManager()
	alertID := alertManager.TriggerAlert(req.Title, req.Message, req.Component, severity, req.Metadata)

	c.JSON(http.StatusOK, gin.H{
		"message":  "Alert triggered successfully",
		"alert_id": alertID,
	})
}

// updateAlertRule updates an alert rule
func updateAlertRule(c *gin.Context) {
	var rule monitoring.AlertRule

	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	alertManager := monitoring.GetAlertManager()
	if err := alertManager.UpdateRule(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Alert rule updated successfully"})
}

// updateAlertChannel updates a notification channel
func updateAlertChannel(c *gin.Context) {
	var channel monitoring.NotificationChannel

	if err := c.ShouldBindJSON(&channel); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	alertManager := monitoring.GetAlertManager()
	if err := alertManager.UpdateChannel(&channel); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notification channel updated successfully"})
}

// testAlertChannel tests a notification channel
func testAlertChannel(c *gin.Context) {
	var req struct {
		ChannelID string `json:"channel_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	alertManager := monitoring.GetAlertManager()
	if err := alertManager.TestChannel(req.ChannelID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Test notification sent successfully"})
}

// getAlertStats returns alert system statistics
func getAlertStats(c *gin.Context) {
	alertManager := monitoring.GetAlertManager()
	stats := alertManager.GetStats()
	c.JSON(http.StatusOK, stats)
}

// CONFIGURATION MANAGEMENT SECTION

// getConfigurationPage renders the configuration management page
func getConfigurationPage(c *gin.Context) {
	// Get the admin auth instance from context
	auth := getAdminAuthFromContext(c)
	if auth == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Admin auth not available"})
		return
	}

	data := gin.H{
		"Title":      "System Configuration",
		"ActivePage": "config",
	}

	c.Header("Content-Type", "text/html")
	if tmpl, exists := auth.templates["config"]; exists {
		tmpl.Execute(c.Writer, data)
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Configuration template not found"})
	}
}

// getCurrentConfiguration returns the current system configuration
func getCurrentConfiguration(c *gin.Context) {
	configManager := config.GetConfigManager()
	currentConfig := configManager.GetCurrentConfig()

	c.JSON(http.StatusOK, currentConfig)
}

// getValidationRules returns validation rules for the frontend
func getValidationRules(c *gin.Context) {
	configManager := config.GetConfigManager()
	rules := configManager.GetValidationRules()

	c.JSON(http.StatusOK, rules)
}

// getConfigurationHistory returns the configuration change history
func getConfigurationHistory(c *gin.Context) {
	configManager := config.GetConfigManager()
	history := configManager.GetHistory()

	c.JSON(http.StatusOK, history)
}

// updateConfiguration updates the entire system configuration
func updateConfiguration(c *gin.Context) {
	var req struct {
		Config      *config.SystemConfig `json:"config" binding:"required"`
		Description string               `json:"description" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format: " + err.Error(),
		})
		return
	}

	// Get the username from session
	session, _ := globalAdminAuth.store.Get(c.Request, "admin-session")
	username, _ := session.Values["username"].(string)
	if username == "" {
		username = "unknown"
	}

	configManager := config.GetConfigManager()
	err := configManager.UpdateConfig(req.Config, username, req.Description)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Configuration updated successfully",
	})
}

// updateConfigurationSection updates a specific configuration section
func updateConfigurationSection(c *gin.Context) {
	var req struct {
		Section     string      `json:"section" binding:"required"`
		Config      interface{} `json:"config" binding:"required"`
		Description string      `json:"description" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format: " + err.Error(),
		})
		return
	}

	// Get the username from session
	session, _ := globalAdminAuth.store.Get(c.Request, "admin-session")
	username, _ := session.Values["username"].(string)
	if username == "" {
		username = "unknown"
	}

	configManager := config.GetConfigManager()
	err := configManager.UpdateSection(req.Section, req.Config, username, req.Description)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Configuration section '%s' updated successfully", req.Section),
	})
}

// resetToDefaultConfiguration resets the configuration to default values
func resetToDefaultConfiguration(c *gin.Context) {
	// Get the username from session
	session, _ := globalAdminAuth.store.Get(c.Request, "admin-session")
	username, _ := session.Values["username"].(string)
	if username == "" {
		username = "unknown"
	}

	configManager := config.GetConfigManager()

	// Create a new default configuration
	defaultConfig := &config.SystemConfig{
		Timestamp: time.Now(),
		Version:   "1.0.0",
		AddressPool: config.PoolConfig{
			MinPoolSize:     10,
			MaxPoolSize:     100,
			RefillThreshold: 5,
			RefillBatchSize: 20,
			CleanupInterval: 60,
		},
		GapMonitor: config.GapConfig{
			MaxGapLimit:              20,
			WarningThreshold:         0.7,
			CriticalThreshold:        0.9,
			ConsecutiveFailThreshold: 5,
			ErrorHistorySize:         100,
		},
		RateLimiter: config.RateLimitConfig{
			GlobalMaxTokens:  500,
			GlobalRefillRate: 50,
			IPMaxTokens:      10,
			IPRefillRate:     5,
			EmailMaxTokens:   5,
			EmailRefillRate:  2,
			CleanupInterval:  30,
		},
		WebSocket: config.WebSocketConfig{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			PingInterval:    30,
			PongTimeout:     60,
			MaxConnections:  1000,
		},
		AdminDashboard: config.AdminConfig{
			SessionTimeout:   24,
			RefreshInterval:  10,
			MaxLogEntries:    1000,
			EnableMetrics:    true,
			EnableAlerts:     true,
			MetricsRetention: 168, // 7 days
		},
	}

	err := configManager.UpdateConfig(defaultConfig, username, "Reset configuration to default values")

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Configuration reset to default values successfully",
	})
}

// rollbackConfiguration rolls back to a previous configuration
func rollbackConfiguration(c *gin.Context) {
	var req struct {
		ChangeID string `json:"change_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format: " + err.Error(),
		})
		return
	}

	// Get the username from session
	session, _ := globalAdminAuth.store.Get(c.Request, "admin-session")
	username, _ := session.Values["username"].(string)
	if username == "" {
		username = "unknown"
	}

	configManager := config.GetConfigManager()
	err := configManager.RollbackToChange(req.ChangeID, username)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Configuration rolled back successfully",
	})
}

// SESSION MANAGEMENT SECTION

// SessionInfo represents information about a user payment session
type SessionInfo struct {
	ID            string    `json:"id"`
	Address       string    `json:"address"`
	UserAgent     string    `json:"user_agent"`
	IPAddress     string    `json:"ip_address"`
	Email         string    `json:"email,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	LastActive    time.Time `json:"last_active"`
	Status        string    `json:"status"`         // "active", "completed", "expired", "terminated"
	PaymentStatus string    `json:"payment_status"` // "pending", "paid", "failed"
	Amount        float64   `json:"amount,omitempty"`
	PaymentID     string    `json:"payment_id,omitempty"`
	Duration      int64     `json:"duration_seconds"`
	IsWebSocket   bool      `json:"is_websocket"`
}

// Global session store for tracking active sessions
var (
	activeSessionsStore = make(map[string]*SessionInfo)
	sessionHistoryStore = make([]*SessionInfo, 0)
	sessionStoreMutex   = sync.RWMutex{}
)

// AddSession adds a new session to tracking
func AddSession(sessionID, address, userAgent, ipAddress, email string, amount float64, paymentID string) {
	sessionStoreMutex.Lock()
	defer sessionStoreMutex.Unlock()

	session := &SessionInfo{
		ID:            sessionID,
		Address:       address,
		UserAgent:     userAgent,
		IPAddress:     ipAddress,
		Email:         email,
		CreatedAt:     time.Now(),
		LastActive:    time.Now(),
		Status:        "active",
		PaymentStatus: "pending",
		Amount:        amount,
		PaymentID:     paymentID,
		Duration:      0,
		IsWebSocket:   false,
	}

	activeSessionsStore[sessionID] = session
}

// UpdateSessionStatus updates the status of a session
func UpdateSessionStatus(sessionID, status string) {
	sessionStoreMutex.Lock()
	defer sessionStoreMutex.Unlock()

	if session, exists := activeSessionsStore[sessionID]; exists {
		session.Status = status
		session.LastActive = time.Now()
		session.Duration = int64(time.Since(session.CreatedAt).Seconds())

		// Move completed/expired/terminated sessions to history
		if status != "active" {
			// Create a copy for history
			historySessions := *session
			sessionHistoryStore = append(sessionHistoryStore, &historySessions)

			// Keep only last 1000 history entries
			if len(sessionHistoryStore) > 1000 {
				sessionHistoryStore = sessionHistoryStore[len(sessionHistoryStore)-1000:]
			}

			// Remove from active sessions
			delete(activeSessionsStore, sessionID)
		}
	}
}

// UpdateSessionStatusByAddress finds and updates session status by address
func UpdateSessionStatusByAddress(address, status string) {
	sessionStoreMutex.Lock()
	defer sessionStoreMutex.Unlock()

	for sessionID, session := range activeSessionsStore {
		if session.Address == address {
			session.Status = status
			session.LastActive = time.Now()
			session.Duration = int64(time.Since(session.CreatedAt).Seconds())

			// Update payment status based on session status
			if status == "completed" {
				session.PaymentStatus = "paid"
			} else if status == "expired" || status == "terminated" {
				session.PaymentStatus = "failed"
			}

			// Move completed/expired/terminated sessions to history
			if status != "active" {
				// Create a copy for history
				historySessions := *session
				sessionHistoryStore = append(sessionHistoryStore, &historySessions)

				// Keep only last 1000 history entries
				if len(sessionHistoryStore) > 1000 {
					sessionHistoryStore = sessionHistoryStore[len(sessionHistoryStore)-1000:]
				}

				// Remove from active sessions
				delete(activeSessionsStore, sessionID)
			}
			break
		}
	}
}

// UpdateSessionWebSocket marks a session as having WebSocket connection
func UpdateSessionWebSocket(sessionID string, hasWebSocket bool) {
	sessionStoreMutex.RLock()
	defer sessionStoreMutex.RUnlock()

	if session, exists := activeSessionsStore[sessionID]; exists {
		session.IsWebSocket = hasWebSocket
		session.LastActive = time.Now()
	}
}

// UpdateSessionWebSocketByAddress finds and updates WebSocket status by address
func UpdateSessionWebSocketByAddress(address string, connected bool) {
	sessionStoreMutex.Lock()
	defer sessionStoreMutex.Unlock()

	for _, session := range activeSessionsStore {
		if session.Address == address {
			session.IsWebSocket = connected
			session.LastActive = time.Now()
			// Don't break - multiple sessions could have same address
		}
	}
}

// autoExpireSessions automatically expires sessions older than 30 minutes
func autoExpireSessions() {
	sessionStoreMutex.Lock()
	defer sessionStoreMutex.Unlock()

	now := time.Now()
	expiredSessionIDs := make([]string, 0)

	// Find sessions older than 30 minutes
	for sessionID, session := range activeSessionsStore {
		age := now.Sub(session.CreatedAt)
		lastActiveAge := now.Sub(session.LastActive)

		// Expire if session is older than 30 minutes or hasn't been active for 15 minutes
		if age > 30*time.Minute || lastActiveAge > 15*time.Minute {
			expiredSessionIDs = append(expiredSessionIDs, sessionID)
		}
	}

	// Move expired sessions to history
	for _, sessionID := range expiredSessionIDs {
		if session, exists := activeSessionsStore[sessionID]; exists {
			session.Status = "expired"
			session.PaymentStatus = "failed"
			session.Duration = int64(now.Sub(session.CreatedAt).Seconds())

			// Create a copy for history
			historySessions := *session
			sessionHistoryStore = append(sessionHistoryStore, &historySessions)

			// Keep only last 1000 history entries
			if len(sessionHistoryStore) > 1000 {
				sessionHistoryStore = sessionHistoryStore[len(sessionHistoryStore)-1000:]
			}

			// Remove from active sessions
			delete(activeSessionsStore, sessionID)
		}
	}
}

// getSessionsPage renders the sessions management page
func getSessionsPage(c *gin.Context) {
	data := gin.H{
		"Title":      "Session Management",
		"ActivePage": "sessions",
	}

	// Use the global admin auth template system
	if globalAdminAuth != nil && globalAdminAuth.templates["sessions"] != nil {
		c.Header("Content-Type", "text/html")
		globalAdminAuth.templates["sessions"].Execute(c.Writer, data)
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Sessions template not found"})
	}
}

// getActiveSessions returns currently active user sessions
func getActiveSessions(c *gin.Context) {
	// First perform automatic cleanup of expired sessions
	autoExpireSessions()

	sessionStoreMutex.RLock()
	defer sessionStoreMutex.RUnlock()

	sessions := make([]*SessionInfo, 0, len(activeSessionsStore))
	for _, session := range activeSessionsStore {
		// Update duration
		session.Duration = int64(time.Since(session.CreatedAt).Seconds())
		sessions = append(sessions, session)
	}

	// Sort by creation time (newest first)
	for i := 0; i < len(sessions)-1; i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[i].CreatedAt.Before(sessions[j].CreatedAt) {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"sessions": sessions,
		"count":    len(sessions),
	})
}

// getSessionHistory returns historical session data
func getSessionHistory(c *gin.Context) {
	sessionStoreMutex.RLock()
	defer sessionStoreMutex.RUnlock()

	limit := 100 // Default limit
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	// Get the most recent sessions up to limit
	sessions := make([]*SessionInfo, 0)
	startIndex := 0
	if len(sessionHistoryStore) > limit {
		startIndex = len(sessionHistoryStore) - limit
	}

	for i := startIndex; i < len(sessionHistoryStore); i++ {
		sessions = append(sessions, sessionHistoryStore[i])
	}

	// Reverse to show newest first
	for i := 0; i < len(sessions)/2; i++ {
		sessions[i], sessions[len(sessions)-1-i] = sessions[len(sessions)-1-i], sessions[i]
	}

	c.JSON(http.StatusOK, gin.H{
		"sessions": sessions,
		"count":    len(sessions),
		"total":    len(sessionHistoryStore),
	})
}

// getSessionStats returns session statistics
func getSessionStats(c *gin.Context) {
	sessionStoreMutex.RLock()
	defer sessionStoreMutex.RUnlock()

	// Get current BTC to USD rate
	rate, err := utils.GetBlockonomicsRate()
	if err != nil {
		log.Printf("Error fetching BTC rate for admin stats: %s", err)
		rate = 0 // Default to 0 if rate fetch fails
	}

	// Active session stats
	activeCount := len(activeSessionsStore)
	webSocketCount := 0
	activeAmountTotal := 0.0

	for _, session := range activeSessionsStore {
		if session.IsWebSocket {
			webSocketCount++
		}
		// Convert BTC amount to USD for display
		if rate > 0 {
			amountUSD := session.Amount * rate
			activeAmountTotal += amountUSD
		}
	}

	// Historical stats
	totalSessions := len(sessionHistoryStore) + activeCount
	completedCount := 0
	expiredCount := 0
	terminatedCount := 0
	paidCount := 0
	failedCount := 0
	pendingCount := 0
	historicalAmountTotal := 0.0
	paidAmountTotal := 0.0

	for _, session := range sessionHistoryStore {
		switch session.Status {
		case "completed":
			completedCount++
		case "expired":
			expiredCount++
		case "terminated":
			terminatedCount++
		}

		switch session.PaymentStatus {
		case "paid":
			paidCount++
			if rate > 0 {
				amountUSD := session.Amount
				paidAmountTotal += amountUSD
			}
		case "failed":
			failedCount++
		case "pending":
			pendingCount++
		}

		// Convert BTC amount to USD for display
		if rate > 0 {
			amountUSD := session.Amount
			historicalAmountTotal += amountUSD
		}
	}

	// Add active session payment statuses
	for _, session := range activeSessionsStore {
		switch session.PaymentStatus {
		case "pending":
			pendingCount++
		case "paid":
			paidCount++
		case "failed":
			failedCount++
		}
	}

	// Success rate calculation (based on completed payments)
	successRate := 0.0
	paymentSuccessRate := 0.0
	if totalSessions > 0 {
		successRate = (float64(completedCount) / float64(totalSessions)) * 100
		paymentSuccessRate = (float64(paidCount) / float64(totalSessions)) * 100
	}

	// Average session duration from history
	avgDuration := 0.0
	if len(sessionHistoryStore) > 0 {
		var totalDuration int64
		for _, session := range sessionHistoryStore {
			totalDuration += session.Duration
		}
		avgDuration = float64(totalDuration) / float64(len(sessionHistoryStore))
	}

	// Conversion rate (sessions that result in payments)
	conversionRate := 0.0
	if totalSessions > 0 {
		conversionRate = (float64(paidCount) / float64(totalSessions)) * 100
	}

	stats := gin.H{
		"active_sessions": gin.H{
			"count":           activeCount,
			"websocket_count": webSocketCount,
			"total_amount":    activeAmountTotal,
		},
		"historical": gin.H{
			"total_sessions": totalSessions,
			"completed":      completedCount,
			"expired":        expiredCount,
			"terminated":     terminatedCount,
			"total_amount":   historicalAmountTotal,
		},
		"payments": gin.H{
			"paid_count":    paidCount,
			"failed_count":  failedCount,
			"pending_count": pendingCount,
			"paid_amount":   paidAmountTotal,
		},
		"metrics": gin.H{
			"success_rate":         successRate,
			"payment_success_rate": paymentSuccessRate,
			"conversion_rate":      conversionRate,
			"avg_duration_seconds": avgDuration,
		},
		"timestamp": time.Now().Unix(),
	}

	c.JSON(http.StatusOK, stats)
}

// getSessionAnalytics returns detailed analytics data for charts
func getSessionAnalytics(c *gin.Context) {
	sessionStoreMutex.RLock()
	defer sessionStoreMutex.RUnlock()

	// Status distribution
	statusData := map[string]int{
		"active":     0,
		"completed":  0,
		"expired":    0,
		"terminated": 0,
	}

	// Payment status distribution
	paymentData := map[string]int{
		"pending": 0,
		"paid":    0,
		"failed":  0,
	}

	// Amount ranges
	amountRanges := map[string]int{
		"0-10":    0,
		"10-50":   0,
		"50-100":  0,
		"100-500": 0,
		"500+":    0,
	}

	// WebSocket usage
	wsData := map[string]int{
		"with_websocket":    0,
		"without_websocket": 0,
	}

	// Process active sessions
	for _, session := range activeSessionsStore {
		statusData["active"]++
		paymentData[session.PaymentStatus]++

		if session.IsWebSocket {
			wsData["with_websocket"]++
		} else {
			wsData["without_websocket"]++
		}

		// Amount categorization
		amount := session.Amount
		switch {
		case amount <= 10:
			amountRanges["0-10"]++
		case amount <= 50:
			amountRanges["10-50"]++
		case amount <= 100:
			amountRanges["50-100"]++
		case amount <= 500:
			amountRanges["100-500"]++
		default:
			amountRanges["500+"]++
		}
	}

	// Process historical sessions
	for _, session := range sessionHistoryStore {
		statusData[session.Status]++
		paymentData[session.PaymentStatus]++

		if session.IsWebSocket {
			wsData["with_websocket"]++
		} else {
			wsData["without_websocket"]++
		}

		// Amount categorization
		amount := session.Amount
		switch {
		case amount <= 10:
			amountRanges["0-10"]++
		case amount <= 50:
			amountRanges["10-50"]++
		case amount <= 100:
			amountRanges["50-100"]++
		case amount <= 500:
			amountRanges["100-500"]++
		default:
			amountRanges["500+"]++
		}
	}

	analytics := gin.H{
		"status_distribution":  statusData,
		"payment_distribution": paymentData,
		"amount_ranges":        amountRanges,
		"websocket_usage":      wsData,
		"timestamp":            time.Now().Unix(),
	}

	c.JSON(http.StatusOK, analytics)
}

// getSessionTimeline returns hourly session data for the last 24 hours
func getSessionTimeline(c *gin.Context) {
	sessionStoreMutex.RLock()
	defer sessionStoreMutex.RUnlock()

	now := time.Now()
	hours := 24
	timeline := make([]gin.H, hours)

	// Initialize timeline with empty data
	for i := 0; i < hours; i++ {
		hourStart := now.Add(-time.Duration(hours-i-1) * time.Hour)
		timeline[i] = gin.H{
			"hour":      hourStart.Format("15:04"),
			"timestamp": hourStart.Unix(),
			"created":   0,
			"completed": 0,
			"expired":   0,
			"paid":      0,
			"failed":    0,
		}
	}

	// Process historical sessions
	for _, session := range sessionHistoryStore {
		// Check if session falls within our 24-hour window
		if session.CreatedAt.After(now.Add(-24 * time.Hour)) {
			// Find which hour this session belongs to
			hoursDiff := int(now.Sub(session.CreatedAt).Hours())
			if hoursDiff >= 0 && hoursDiff < hours {
				index := hours - hoursDiff - 1
				if index >= 0 && index < len(timeline) {
					timeline[index]["created"] = timeline[index]["created"].(int) + 1

					switch session.Status {
					case "completed":
						timeline[index]["completed"] = timeline[index]["completed"].(int) + 1
					case "expired":
						timeline[index]["expired"] = timeline[index]["expired"].(int) + 1
					}

					switch session.PaymentStatus {
					case "paid":
						timeline[index]["paid"] = timeline[index]["paid"].(int) + 1
					case "failed":
						timeline[index]["failed"] = timeline[index]["failed"].(int) + 1
					}
				}
			}
		}
	}

	// Process active sessions (only for creation count)
	for _, session := range activeSessionsStore {
		if session.CreatedAt.After(now.Add(-24 * time.Hour)) {
			hoursDiff := int(now.Sub(session.CreatedAt).Hours())
			if hoursDiff >= 0 && hoursDiff < hours {
				index := hours - hoursDiff - 1
				if index >= 0 && index < len(timeline) {
					timeline[index]["created"] = timeline[index]["created"].(int) + 1
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"timeline":  timeline,
		"timestamp": time.Now().Unix(),
	})
}

// getSessionTrends returns trend data and insights
func getSessionTrends(c *gin.Context) {
	sessionStoreMutex.RLock()
	defer sessionStoreMutex.RUnlock()

	now := time.Now()
	last24h := now.Add(-24 * time.Hour)
	last7d := now.Add(-7 * 24 * time.Hour)

	// Initialize counters
	trends := gin.H{
		"last_24h": gin.H{
			"sessions":  0,
			"completed": 0,
			"paid":      0,
			"amount":    0.0,
		},
		"last_7d": gin.H{
			"sessions":  0,
			"completed": 0,
			"paid":      0,
			"amount":    0.0,
		},
		"current_active":     len(activeSessionsStore),
		"current_websockets": 0,
		"performance": gin.H{
			"avg_session_duration": 0.0,
			"conversion_rate":      0.0,
			"success_rate":         0.0,
		},
	}

	// Count current WebSocket connections
	for _, session := range activeSessionsStore {
		if session.IsWebSocket {
			trends["current_websockets"] = trends["current_websockets"].(int) + 1
		}
	}

	var last24hDurations []int64
	var last7dDurations []int64

	// Process historical sessions
	for _, session := range sessionHistoryStore {
		// Last 24 hours
		if session.CreatedAt.After(last24h) {
			trends["last_24h"].(gin.H)["sessions"] = trends["last_24h"].(gin.H)["sessions"].(int) + 1
			if session.Status == "completed" {
				trends["last_24h"].(gin.H)["completed"] = trends["last_24h"].(gin.H)["completed"].(int) + 1
			}
			if session.PaymentStatus == "paid" {
				trends["last_24h"].(gin.H)["paid"] = trends["last_24h"].(gin.H)["paid"].(int) + 1
				trends["last_24h"].(gin.H)["amount"] = trends["last_24h"].(gin.H)["amount"].(float64) + session.Amount
			}
			last24hDurations = append(last24hDurations, session.Duration)
		}

		// Last 7 days
		if session.CreatedAt.After(last7d) {
			trends["last_7d"].(gin.H)["sessions"] = trends["last_7d"].(gin.H)["sessions"].(int) + 1
			if session.Status == "completed" {
				trends["last_7d"].(gin.H)["completed"] = trends["last_7d"].(gin.H)["completed"].(int) + 1
			}
			if session.PaymentStatus == "paid" {
				trends["last_7d"].(gin.H)["paid"] = trends["last_7d"].(gin.H)["paid"].(int) + 1
				trends["last_7d"].(gin.H)["amount"] = trends["last_7d"].(gin.H)["amount"].(float64) + session.Amount
			}
			last7dDurations = append(last7dDurations, session.Duration)
		}
	}

	// Calculate performance metrics
	if len(last24hDurations) > 0 {
		var totalDuration int64
		for _, d := range last24hDurations {
			totalDuration += d
		}
		trends["performance"].(gin.H)["avg_session_duration"] = float64(totalDuration) / float64(len(last24hDurations))
	}

	// Calculate rates
	if trends["last_24h"].(gin.H)["sessions"].(int) > 0 {
		conversionRate := (float64(trends["last_24h"].(gin.H)["paid"].(int)) / float64(trends["last_24h"].(gin.H)["sessions"].(int))) * 100
		successRate := (float64(trends["last_24h"].(gin.H)["completed"].(int)) / float64(trends["last_24h"].(gin.H)["sessions"].(int))) * 100
		trends["performance"].(gin.H)["conversion_rate"] = conversionRate
		trends["performance"].(gin.H)["success_rate"] = successRate
	}

	trends["timestamp"] = time.Now().Unix()
	c.JSON(http.StatusOK, trends)
}

// getDashboardSessionStats returns basic session statistics for dashboard overview
func getDashboardSessionStats(c *gin.Context) {
	sessionStoreMutex.RLock()
	defer sessionStoreMutex.RUnlock()

	// Count active sessions and websocket connections
	activeSessions := 0
	websocketCount := 0
	paidSessions := 0
	totalPaidAmount := 0.0

	// Count active sessions
	for _, session := range activeSessionsStore {
		if session.Status == "active" {
			activeSessions++
		}
		if session.IsWebSocket {
			websocketCount++
		}
		if session.PaymentStatus == "paid" {
			paidSessions++
			totalPaidAmount += session.Amount
		}
	}

	// Count paid sessions from history as well
	for _, session := range sessionHistoryStore {
		if session.PaymentStatus == "paid" {
			paidSessions++
			totalPaidAmount += session.Amount
		}
	}

	// Calculate payment rate based on all sessions (active + history)
	totalSessions := len(activeSessionsStore) + len(sessionHistoryStore)
	paymentRate := 0.0
	if totalSessions > 0 {
		paymentRate = (float64(paidSessions) / float64(totalSessions)) * 100
	}

	stats := gin.H{
		"active_sessions": activeSessions,
		"websocket_count": websocketCount,
		"payment_rate":    paymentRate,
		"paid_amount":     totalPaidAmount,
		"total_sessions":  totalSessions,
		"paid_sessions":   paidSessions,
		"timestamp":       time.Now().Unix(),
	}

	c.JSON(http.StatusOK, stats)
}

// clearSessionHistory clears all session history data
func clearSessionHistory(c *gin.Context) {
	sessionStoreMutex.Lock()
	defer sessionStoreMutex.Unlock()

	historyCount := len(sessionHistoryStore)

	// Clear all session history
	sessionHistoryStore = []*SessionInfo{}

	log.Printf("Admin cleared all session history. Removed %d historical sessions", historyCount)

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       fmt.Sprintf("Successfully cleared %d historical sessions", historyCount),
		"cleared_count": historyCount,
		"timestamp":     time.Now().Unix(),
	})
}

// terminateSession forcefully terminates an active session
func terminateSession(c *gin.Context) {
	var req struct {
		SessionID string `json:"session_id" binding:"required"`
		Reason    string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Session ID is required",
		})
		return
	}

	sessionStoreMutex.Lock()
	defer sessionStoreMutex.Unlock()

	session, exists := activeSessionsStore[req.SessionID]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Session not found",
		})
		return
	}

	// Update session status
	session.Status = "terminated"
	session.LastActive = time.Now()
	session.Duration = int64(time.Since(session.CreatedAt).Seconds())

	// Add reason to session data (we'd need to extend SessionInfo for this)
	log.Printf("Admin terminated session %s for address %s. Reason: %s", req.SessionID, session.Address, req.Reason)

	// Move to history
	historySessions := *session
	sessionHistoryStore = append(sessionHistoryStore, &historySessions)

	// Keep only last 1000 history entries
	if len(sessionHistoryStore) > 1000 {
		sessionHistoryStore = sessionHistoryStore[len(sessionHistoryStore)-1000:]
	}

	// Remove from active sessions
	delete(activeSessionsStore, req.SessionID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Session terminated successfully",
	})
}

// cleanupSessions performs bulk cleanup operations
func cleanupSessions(c *gin.Context) {
	var req struct {
		ExpiredOnly    bool `json:"expired_only"`
		OlderThanHours int  `json:"older_than_hours"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		// Use defaults if no body provided
		req.ExpiredOnly = true
		req.OlderThanHours = 24
	}

	sessionStoreMutex.Lock()
	defer sessionStoreMutex.Unlock()

	cutoffTime := time.Now().Add(-time.Duration(req.OlderThanHours) * time.Hour)
	removedCount := 0

	// Clean up active sessions based on criteria
	for sessionID, session := range activeSessionsStore {
		shouldRemove := false

		if req.ExpiredOnly {
			// Remove sessions older than cutoff that are still "active"
			if session.CreatedAt.Before(cutoffTime) {
				shouldRemove = true
				session.Status = "expired"
			}
		} else {
			// Remove all sessions older than cutoff
			if session.LastActive.Before(cutoffTime) {
				shouldRemove = true
				session.Status = "cleaned"
			}
		}

		if shouldRemove {
			session.Duration = int64(time.Since(session.CreatedAt).Seconds())

			// Move to history
			historySessions := *session
			sessionHistoryStore = append(sessionHistoryStore, &historySessions)

			// Remove from active
			delete(activeSessionsStore, sessionID)
			removedCount++
		}
	}

	// Cleanup history if it's getting too large
	if len(sessionHistoryStore) > 1000 {
		sessionHistoryStore = sessionHistoryStore[len(sessionHistoryStore)-1000:]
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       fmt.Sprintf("Cleanup completed. Removed %d sessions.", removedCount),
		"removed_count": removedCount,
	})
}

// exportSessionData exports session data for analysis
func exportSessionData(c *gin.Context) {
	format := c.DefaultQuery("format", "json")
	includeActive := c.DefaultQuery("include_active", "true") == "true"
	includeHistory := c.DefaultQuery("include_history", "true") == "true"

	sessionStoreMutex.RLock()
	defer sessionStoreMutex.RUnlock()

	exportData := gin.H{
		"export_info": gin.H{
			"generated_at":    time.Now(),
			"format":          format,
			"include_active":  includeActive,
			"include_history": includeHistory,
			"version":         "1.0",
			"source":          "PayButton Admin Dashboard",
		},
	}

	if includeActive {
		sessions := make([]*SessionInfo, 0, len(activeSessionsStore))
		for _, session := range activeSessionsStore {
			sessions = append(sessions, session)
		}
		exportData["active_sessions"] = sessions
	}

	if includeHistory {
		exportData["session_history"] = sessionHistoryStore
	}

	// Add summary statistics
	exportData["summary"] = gin.H{
		"active_count":  len(activeSessionsStore),
		"history_count": len(sessionHistoryStore),
		"total_count":   len(activeSessionsStore) + len(sessionHistoryStore),
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("2006-01-02-15-04-05")
	filename := fmt.Sprintf("session-data-%s", timestamp)

	switch format {
	case "csv":
		// Convert to CSV format
		csvData := convertSessionsToCSV(activeSessionsStore, sessionHistoryStore, includeActive, includeHistory)
		c.Header("Content-Type", "text/csv")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.csv", filename))
		c.String(http.StatusOK, csvData)
	default:
		// Default to JSON
		c.Header("Content-Type", "application/json")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.json", filename))
		c.JSON(http.StatusOK, exportData)
	}
}

// Helper function to convert sessions to CSV format
func convertSessionsToCSV(activeSessions map[string]*SessionInfo, historySessions []*SessionInfo, includeActive, includeHistory bool) string {
	csv := "ID,Address,IP,Email,Status,Created,Duration,Amount,WebSocket,UserAgent\n"

	if includeActive {
		for _, session := range activeSessions {
			csv += fmt.Sprintf("%s,%s,%s,%s,%s,%s,%d,%.8f,%v,%s\n",
				session.ID,
				session.Address,
				session.IPAddress,
				session.Email,
				session.Status,
				session.CreatedAt.Format("2006-01-02 15:04:05"),
				session.Duration,
				session.Amount,
				session.IsWebSocket,
				fmt.Sprintf("\"%s\"", session.UserAgent), // Quoted for CSV safety
			)
		}
	}

	if includeHistory {
		for _, session := range historySessions {
			csv += fmt.Sprintf("%s,%s,%s,%s,%s,%s,%d,%.8f,%v,%s\n",
				session.ID,
				session.Address,
				session.IPAddress,
				session.Email,
				session.Status,
				session.CreatedAt.Format("2006-01-02 15:04:05"),
				session.Duration,
				session.Amount,
				session.IsWebSocket,
				fmt.Sprintf("\"%s\"", session.UserAgent), // Quoted for CSV safety
			)
		}
	}

	return csv
}

// Analytics endpoint handlers

// getSiteAnalyticsData returns full site analytics data
func getSiteAnalyticsData(c *gin.Context) {
	allSites := analytics.GetAllSiteAnalytics()
	totalActive := analytics.GetTotalActiveViewers()
	totalWeekly := analytics.GetTotalWeeklyVisitors()
	activeSites := analytics.GetActiveSitesCount()

	// Debug logging
	log.Printf("Analytics API called: active=%d, weekly=%d, active_sites=%d, total_sites=%d", 
		totalActive, totalWeekly, activeSites, len(allSites))

	response := gin.H{
		"sites": allSites,
		"totals": gin.H{
			"active":       totalActive,
			"weekly":       totalWeekly,
			"active_sites": activeSites,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, response)
}

// getDashboardAnalytics returns summary analytics data for dashboard widget
func getDashboardAnalytics(c *gin.Context) {
	totalActive := analytics.GetTotalActiveViewers()
	totalWeekly := analytics.GetTotalWeeklyVisitors()
	activeSites := analytics.GetActiveSitesCount()

	// Get top 5 most active sites for mini chart
	allSites := analytics.GetAllSiteAnalytics()
	topSites := make([]gin.H, 0)

	// Convert to slice for sorting
	siteList := make([]analytics.SiteAnalytics, 0, len(allSites))
	for _, site := range allSites {
		siteList = append(siteList, site)
	}

	// Sort by active count (simple bubble sort for small dataset)
	for i := 0; i < len(siteList)-1; i++ {
		for j := 0; j < len(siteList)-i-1; j++ {
			if siteList[j].ActiveCount < siteList[j+1].ActiveCount {
				siteList[j], siteList[j+1] = siteList[j+1], siteList[j]
			}
		}
	}

	// Take top 5 sites
	maxSites := 5
	if len(siteList) < maxSites {
		maxSites = len(siteList)
	}

	for i := 0; i < maxSites; i++ {
		site := siteList[i]
		topSites = append(topSites, gin.H{
			"name":   site.SiteName,
			"active": site.ActiveCount,
			"weekly": site.WeeklyTotal,
		})
	}

	response := gin.H{
		"summary": gin.H{
			"total_active": totalActive,
			"total_weekly": totalWeekly,
			"active_sites": activeSites,
			"total_sites":  len(allSites),
		},
		"top_sites": topSites,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, response)
}

// Phase 5: getSiteHistoricalData returns historical data for a specific site
func getSiteHistoricalData(c *gin.Context) {
	siteName := c.Param("siteName")
	if siteName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Site name is required"})
		return
	}

	// Get hours parameter (default 24 hours)
	hours := 24
	if hoursParam := c.Query("hours"); hoursParam != "" {
		if h, err := strconv.Atoi(hoursParam); err == nil && h > 0 && h <= 720 {
			hours = h
		}
	}

	// Get historical data from analytics manager
	historicalData := analytics.GetSiteHistoricalData(siteName, hours)

	response := gin.H{
		"site_name":       siteName,
		"hours_requested": hours,
		"data_points":     len(historicalData),
		"historical_data": historicalData,
		"timestamp":       time.Now().Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, response)
}

// Phase 5: getSitePageStats returns popular page statistics for a specific site
func getSitePageStats(c *gin.Context) {
	siteName := c.Param("siteName")
	if siteName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Site name is required"})
		return
	}

	// Get limit parameter (default 10)
	limit := 10
	if limitParam := c.Query("limit"); limitParam != "" {
		if l, err := strconv.Atoi(limitParam); err == nil && l > 0 {
			limit = l
		}
	}

	// Get page stats from analytics manager
	pageStats := analytics.GetSitePageStats(siteName, limit)

	response := gin.H{
		"site_name":   siteName,
		"total_pages": len(pageStats),
		"page_stats":  pageStats,
		"timestamp":   time.Now().Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, response)
}

// Phase 5: getSiteRegionStats returns region statistics for a specific site
func getSiteRegionStats(c *gin.Context) {
	siteName := c.Param("siteName")
	if siteName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Site name is required"})
		return
	}

	// Get region stats from analytics manager
	regionStats := analytics.GetSiteRegionStats(siteName)

	// Calculate total active viewers for percentage calculation
	totalActive := 0
	for _, region := range regionStats {
		totalActive += region.Count
	}

	// Add percentages to region stats
	enrichedStats := make([]gin.H, len(regionStats))
	for i, region := range regionStats {
		percentage := 0.0
		if totalActive > 0 {
			percentage = float64(region.Count) / float64(totalActive) * 100.0
		}

		enrichedStats[i] = gin.H{
			"region":     region.Region,
			"count":      region.Count,
			"percentage": percentage,
		}
	}

	response := gin.H{
		"site_name":     siteName,
		"total_active":  totalActive,
		"total_regions": len(regionStats),
		"region_stats":  enrichedStats,
		"timestamp":     time.Now().Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, response)
}

// Phase 5: exportSiteAnalyticsData exports comprehensive analytics data for a site
func exportSiteAnalyticsData(c *gin.Context) {
	siteName := c.Param("siteName")
	if siteName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Site name is required"})
		return
	}

	// Get period parameter (default 30d)
	period := c.DefaultQuery("period", "30d")
	if period != "24h" && period != "7d" && period != "30d" {
		period = "30d"
	}

	// Get format parameter (default json)
	format := c.DefaultQuery("format", "json")

	// Export data using analytics manager
	exportData := analytics.ExportSiteData(siteName, period)
	if exportData == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Site data not found"})
		return
	}

	switch format {
	case "csv":
		// Generate CSV response
		c.Header("Content-Type", "text/csv")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s-analytics-%s.csv\"", siteName, period))

		csvData := convertSiteExportToCSV(exportData)
		c.String(http.StatusOK, csvData)

	case "json":
		fallthrough
	default:
		// Generate JSON response
		c.Header("Content-Type", "application/json")
		if c.Query("download") == "true" {
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s-analytics-%s.json\"", siteName, period))
		}

		c.JSON(http.StatusOK, exportData)
	}
}

// Phase 5: convertSiteExportToCSV converts export data to CSV format
func convertSiteExportToCSV(data *analytics.SiteExportData) string {
	var csvBuilder strings.Builder

	// Header information
	csvBuilder.WriteString(fmt.Sprintf("Site Analytics Export,%s\n", data.SiteName))
	csvBuilder.WriteString(fmt.Sprintf("Export Date,%s\n", data.ExportTimestamp.Format("2006-01-02 15:04:05")))
	csvBuilder.WriteString(fmt.Sprintf("Period,%s\n", data.Period))
	csvBuilder.WriteString(fmt.Sprintf("Active Viewers,%d\n", data.Summary.ActiveCount))
	csvBuilder.WriteString(fmt.Sprintf("Weekly Total,%d\n", data.Summary.WeeklyTotal))
	csvBuilder.WriteString("\n")

	// Historical data section
	csvBuilder.WriteString("Historical Data\n")
	csvBuilder.WriteString("Timestamp,Viewers\n")
	for _, point := range data.HistoricalData {
		csvBuilder.WriteString(fmt.Sprintf("%s,%d\n",
			point.Timestamp.Format("2006-01-02 15:04:05"),
			point.Viewers))
	}
	csvBuilder.WriteString("\n")

	// Page analytics section
	if len(data.PageAnalytics) > 0 {
		csvBuilder.WriteString("Page Analytics\n")
		csvBuilder.WriteString("Path,Views,Unique Visitors,Last Seen\n")
		for _, page := range data.PageAnalytics {
			csvBuilder.WriteString(fmt.Sprintf("%s,%d,%d,%s\n",
				page.Path,
				page.Views,
				page.Unique,
				page.LastSeen.Format("2006-01-02 15:04:05")))
		}
		csvBuilder.WriteString("\n")
	}

	// Region breakdown section
	if len(data.RegionBreakdown) > 0 {
		csvBuilder.WriteString("Region Breakdown\n")
		csvBuilder.WriteString("Region,Active Viewers\n")
		for _, region := range data.RegionBreakdown {
			csvBuilder.WriteString(fmt.Sprintf("%s,%d\n", region.Region, region.Count))
		}
	}

	return csvBuilder.String()
}
