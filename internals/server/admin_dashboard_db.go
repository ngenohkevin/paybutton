package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ngenohkevin/paybutton/internals/database"
	"github.com/ngenohkevin/paybutton/internals/db"
	"github.com/ngenohkevin/paybutton/internals/monitoring"
	"github.com/ngenohkevin/paybutton/internals/payment_processing"
)

// DashboardDataDB represents database-backed dashboard data
type DashboardDataDB struct {
	// Overview statistics
	TotalPayments      int64   `json:"total_payments"`
	CompletedPayments  int64   `json:"completed_payments"`
	PendingPayments    int64   `json:"pending_payments"`
	ConfirmingPayments int64   `json:"confirming_payments"`
	ExpiredPayments    int64   `json:"expired_payments"`
	FailedPayments     int64   `json:"failed_payments"`
	TotalBTCReceived   float64 `json:"total_btc_received"`
	TotalUSDReceived   float64 `json:"total_usd_received"`
	AvgBTCPerPayment   float64 `json:"avg_btc_per_payment"`
	AvgUSDPerPayment   float64 `json:"avg_usd_per_payment"`
	TotalSites         int64   `json:"total_sites"`
	TotalAddressesUsed int64   `json:"total_addresses_used"`
	PaymentsLast24h    int64   `json:"payments_last_24h"`
	PaymentsLastHour   int64   `json:"payments_last_hour"`
	CompletedLast24h   int64   `json:"completed_last_24h"`

	// Today's statistics
	TotalToday         int64   `json:"total_today"`
	CompletedToday     int64   `json:"completed_today"`
	PendingToday       int64   `json:"pending_today"`
	BTCToday           float64 `json:"btc_today"`
	USDToday           float64 `json:"usd_today"`
	ActiveSitesToday   int64   `json:"active_sites_today"`
	UniqueCustomersToday int64 `json:"unique_customers_today"`

	// System health (from existing components - keep in memory)
	PoolStats          payment_processing.PoolStats     `json:"pool_stats"`
	GapStats           map[string]interface{}           `json:"gap_stats"`
	RateLimitStats     map[string]interface{}           `json:"rate_limit_stats"`
	ResourceStats      map[string]interface{}           `json:"resource_stats"`

	// Recent activity
	RecentPayments     []db.GetRecentPaymentsAllSitesRow `json:"recent_payments"`
	ActivePendingPayments []db.GetActivePendingPaymentsRow `json:"active_pending_payments"`
	SiteBreakdown      []db.GetSiteBreakdownRow         `json:"site_breakdown"`

	// Analytics data (still in memory as requested)
	AnalyticsSummary   interface{} `json:"analytics_summary,omitempty"`

	// Session info (still in memory as requested)
	ActiveSessionsCount int `json:"active_sessions_count"`
	SessionsLast24h    int `json:"sessions_last_24h"`
}

// GetDashboardDataFromDB retrieves all dashboard data from the database
func GetDashboardDataFromDB(ctx context.Context) (*DashboardDataDB, error) {
	if database.Queries == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	data := &DashboardDataDB{}

	// Get overview statistics
	overview, err := database.Queries.GetDashboardOverview(ctx)
	if err != nil {
		log.Printf("Error fetching dashboard overview: %v", err)
		return nil, fmt.Errorf("failed to fetch dashboard overview: %w", err)
	}

	data.TotalPayments = overview.TotalPayments
	data.CompletedPayments = overview.CompletedPayments
	data.PendingPayments = overview.PendingPayments
	data.ConfirmingPayments = overview.ConfirmingPayments
	data.ExpiredPayments = overview.ExpiredPayments
	data.FailedPayments = overview.FailedPayments

	// Convert interface{} to float64
	data.TotalBTCReceived = interfaceToFloat64(overview.TotalBtcReceived)
	data.TotalUSDReceived = interfaceToFloat64(overview.TotalUsdReceived)
	data.AvgBTCPerPayment = interfaceToFloat64(overview.AvgBtcPerPayment)
	data.AvgUSDPerPayment = interfaceToFloat64(overview.AvgUsdPerPayment)

	data.TotalSites = overview.TotalSites
	data.TotalAddressesUsed = overview.TotalAddressesUsed
	data.PaymentsLast24h = overview.PaymentsLast24h
	data.PaymentsLastHour = overview.PaymentsLastHour
	data.CompletedLast24h = overview.CompletedLast24h

	// Get today's statistics
	todayStats, err := database.Queries.GetTodayStatistics(ctx)
	if err != nil {
		log.Printf("Error fetching today's statistics: %v", err)
	} else {
		data.TotalToday = todayStats.TotalToday
		data.CompletedToday = todayStats.CompletedToday
		data.PendingToday = todayStats.PendingToday

		data.BTCToday = interfaceToFloat64(todayStats.BtcToday)
		data.USDToday = interfaceToFloat64(todayStats.UsdToday)

		data.ActiveSitesToday = todayStats.ActiveSitesToday
		data.UniqueCustomersToday = todayStats.UniqueCustomersToday
	}

	// Get recent payments (last 10)
	recentPayments, err := database.Queries.GetRecentPaymentsAllSites(ctx, db.GetRecentPaymentsAllSitesParams{
		Limit:  10,
		Offset: 0,
	})
	if err != nil {
		log.Printf("Error fetching recent payments: %v", err)
	} else {
		data.RecentPayments = recentPayments
	}

	// Get active pending payments
	activePending, err := database.Queries.GetActivePendingPayments(ctx)
	if err != nil {
		log.Printf("Error fetching active pending payments: %v", err)
	} else {
		data.ActivePendingPayments = activePending
	}

	// Get site breakdown
	siteBreakdown, err := database.Queries.GetSiteBreakdown(ctx)
	if err != nil {
		log.Printf("Error fetching site breakdown: %v", err)
	} else {
		data.SiteBreakdown = siteBreakdown
	}

	// Get system component stats
	gap := payment_processing.GetGapMonitor()
	limiter := payment_processing.GetRateLimiter()
	resourceMonitor := monitoring.GetResourceMonitor()

	// Get pool stats from database instead of in-memory
	poolStatsDB, err := GetPoolStatsFromDB(ctx)
	if err != nil {
		log.Printf("Error fetching pool stats from DB: %v", err)
		// Fallback to empty stats if DB fetch fails
		data.PoolStats = payment_processing.PoolStats{}
	} else {
		// Convert database stats to PoolStats format for template compatibility
		data.PoolStats = payment_processing.PoolStats{
			TotalGenerated:  poolStatsDB.TotalGenerated,
			TotalUsed:       poolStatsDB.TotalUsed,
			TotalRecycled:   poolStatsDB.TotalRecycled,
			CurrentPoolSize: poolStatsDB.CurrentPoolSize,
		}
		log.Printf("DEBUG: Pool stats from DB - TotalGenerated=%d, CurrentPoolSize=%d, TotalUsed=%d, TotalRecycled=%d",
			poolStatsDB.TotalGenerated, poolStatsDB.CurrentPoolSize, poolStatsDB.TotalUsed, poolStatsDB.TotalRecycled)
	}
	if gap != nil {
		data.GapStats = gap.GetStats()
	}
	if limiter != nil {
		data.RateLimitStats = limiter.GetStats()
	}
	if resourceMonitor != nil {
		data.ResourceStats = resourceMonitor.GetStats() // Already returns map[string]interface{}
	}

	// Get session count (sessions stay in memory as requested)
	sessionStoreMutex.RLock()
	data.ActiveSessionsCount = len(activeSessionsStore)

	// Count sessions in last 24h from history
	count24h := 0
	cutoff := time.Now().Add(-24 * time.Hour)
	if sessionHistoryStore != nil {
		for _, session := range sessionHistoryStore {
			if session != nil && !session.CreatedAt.IsZero() && session.CreatedAt.After(cutoff) {
				count24h++
			}
		}
	}
	data.SessionsLast24h = count24h + data.ActiveSessionsCount
	sessionStoreMutex.RUnlock()

	return data, nil
}

// DashboardHandlerDB is the new database-backed dashboard handler
func (auth *AdminAuth) DashboardHandlerDB(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in DashboardHandlerDB: %v", r)
			// Print stack trace
			debug.PrintStack()
			c.String(http.StatusInternalServerError, "Dashboard error: %v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Printf("DashboardHandlerDB: Starting to load dashboard data")

	// Get dashboard data from database
	dashboardData, err := GetDashboardDataFromDB(ctx)
	if err != nil {
		log.Printf("Error loading dashboard data: %v", err)
		c.String(http.StatusInternalServerError, "Failed to load dashboard data from database: %v", err)
		return
	}

	log.Printf("DashboardHandlerDB: Successfully loaded dashboard data")

	// Use the existing dashboard template (not Gin's HTML renderer)
	c.Header("Content-Type", "text/html")
	err = auth.templates["dashboard"].Execute(c.Writer, gin.H{
		"Title":      "Dashboard (DB)",
		"ActivePage": "dashboard",
		"Data":       dashboardData,
	})

	if err != nil {
		log.Printf("Error rendering dashboard template: %v", err)
		c.String(http.StatusInternalServerError, "Failed to render template: %v", err)
		return
	}

	log.Printf("DashboardHandlerDB: Template rendered successfully")
}

// GetDashboardStatsAPI returns dashboard statistics as JSON
func GetDashboardStatsAPI(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dashboardData, err := GetDashboardDataFromDB(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to load dashboard data",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, dashboardData)
}

// interfaceToFloat64 safely converts interface{} to float64
func interfaceToFloat64(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		// Try parsing string as float
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return 0
}
