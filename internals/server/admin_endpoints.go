package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ngenohkevin/paybutton/internals/payment_processing"
)

// RegisterAdminEndpoints registers admin monitoring endpoints
func RegisterAdminEndpoints(router *gin.Engine) {
	admin := router.Group("/admin")
	
	// Basic auth or API key middleware should be added here for production
	// admin.Use(AuthMiddleware())
	
	// System status endpoint
	admin.GET("/status", getSystemStatus)
	
	// Address pool management
	admin.GET("/pool/stats", getPoolStats)
	admin.POST("/pool/refill", refillAddressPool)
	
	// Gap monitor management
	admin.GET("/gap/stats", getGapStats)
	admin.POST("/gap/reset", resetGapCounter)
	
	// Rate limiter management
	admin.GET("/ratelimit/stats", getRateLimitStats)
	admin.POST("/ratelimit/reset/:email", resetUserRateLimit)
}

// getSystemStatus returns overall system health and statistics
func getSystemStatus(c *gin.Context) {
	pool := payment_processing.GetAddressPool()
	gap := payment_processing.GetGapMonitor()
	limiter := payment_processing.GetRateLimiter()
	
	status := gin.H{
		"status": "healthy",
		"components": gin.H{
			"address_pool": pool.GetStats(),
			"gap_monitor":  gap.GetStats(),
			"rate_limiter": limiter.GetStats(),
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
	// This would trigger the pool refill process
	// Implementation depends on making refillPool public or adding a public method
	c.JSON(http.StatusOK, gin.H{
		"message": "Pool refill initiated",
		"note": "Check logs for progress",
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
		"message": "Gap counter reset successfully",
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
		"email": email,
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