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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ngenohkevin/paybutton/internals/database"
	"github.com/ngenohkevin/paybutton/internals/db"
)

// PaymentsPageData represents data for the payments tracking page
type PaymentsPageData struct {
	Payments       []PaymentDisplay `json:"payments"`
	Stats          PaymentStats     `json:"stats"`
	Filters        PaymentFilters   `json:"filters"`
	Pagination     PaginationInfo   `json:"pagination"`
	StatusCounts   []StatusCount    `json:"status_counts"`
	SiteBreakdown  []SiteStats      `json:"site_breakdown"`
}

// PaymentDisplay represents a payment for display
type PaymentDisplay struct {
	PaymentID        string     `json:"payment_id"`
	Address          string     `json:"address"`
	Site             string     `json:"site"`
	AmountBTC        float64    `json:"amount_btc"`
	AmountUSD        float64    `json:"amount_usd"`
	Currency         string     `json:"currency"`
	Status           string     `json:"status"`
	Confirmations    int32      `json:"confirmations"`
	RequiredConfs    int32      `json:"required_confirmations"`
	Email            string     `json:"email"`
	TxHash           string     `json:"tx_hash"`
	CreatedAt        time.Time  `json:"created_at"`
	ConfirmedAt      *time.Time `json:"confirmed_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	TimeUntilExpiry  string     `json:"time_until_expiry,omitempty"`
	StatusBadgeClass string     `json:"status_badge_class"`
	GenerationCount  int32      `json:"generation_count,omitempty"`
	FirstCreatedAt   *time.Time `json:"first_created_at,omitempty"`
}

// PaymentStats represents payment statistics
type PaymentStats struct {
	TotalPayments     int64   `json:"total_payments"`
	CompletedPayments int64   `json:"completed_payments"`
	PendingPayments   int64   `json:"pending_payments"`
	ExpiredPayments   int64   `json:"expired_payments"`
	TotalBTC          float64 `json:"total_btc"`
	TotalUSD          float64 `json:"total_usd"`
	AvgBTC            float64 `json:"avg_btc"`
	AvgUSD            float64 `json:"avg_usd"`
	ConversionRate    float64 `json:"conversion_rate"`
}

// PaymentFilters represents active filters
type PaymentFilters struct {
	Site       string `json:"site,omitempty"`
	Status     string `json:"status,omitempty"`
	Search     string `json:"search,omitempty"`
	StartDate  string `json:"start_date,omitempty"`
	EndDate    string `json:"end_date,omitempty"`
}

// PaginationInfo represents pagination details
type PaginationInfo struct {
	CurrentPage int   `json:"current_page"`
	TotalPages  int   `json:"total_pages"`
	PageSize    int   `json:"page_size"`
	TotalItems  int64 `json:"total_items"`
	HasNext     bool  `json:"has_next"`
	HasPrev     bool  `json:"has_prev"`
}

// StatusCount represents count of payments by status
type StatusCount struct {
	Status   string  `json:"status"`
	Count    int64   `json:"count"`
	TotalUSD float64 `json:"total_usd"`
}

// SiteStats represents statistics per site
type SiteStats struct {
	Site              string    `json:"site"`
	TotalPayments     int64     `json:"total_payments"`
	CompletedPayments int64     `json:"completed_payments"`
	PendingPayments   int64     `json:"pending_payments"`
	TotalBTC          float64   `json:"total_btc"`
	TotalUSD          float64   `json:"total_usd"`
	LastPaymentAt     time.Time `json:"last_payment_at"`
}

// getPaymentsPageDB handles the payments tracking page with database backend
func getPaymentsPageDB(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in getPaymentsPageDB: %v", r)
			debug.PrintStack()
			c.String(http.StatusInternalServerError, "Payments page error: %v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	log.Printf("getPaymentsPageDB: Starting to load payments data")

	// Parse filters and pagination
	filters := parsePaymentFilters(c)
	page := parseInt(c.Query("page"), 1)
	pageSize := parseInt(c.Query("page_size"), 50)

	// Get payments data
	paymentsData, err := getPaymentsDataFromDB(ctx, filters, page, pageSize)
	if err != nil {
		log.Printf("Error loading payments data: %v", err)
		c.String(http.StatusInternalServerError, "Failed to load payments data: %v", err)
		return
	}

	log.Printf("getPaymentsPageDB: Successfully loaded payments data")

	// Check if HTMX request (for partial updates)
	if c.GetHeader("HX-Request") == "true" {
		// For HTMX, we might need a partial template - for now just return JSON
		c.JSON(http.StatusOK, paymentsData)
		return
	}

	// Full page render using custom template system
	c.Header("Content-Type", "text/html")
	err = globalAdminAuth.templates["payments"].Execute(c.Writer, gin.H{
		"Title":      "Payment Tracking (DB)",
		"ActivePage": "payments",
		"Data":       paymentsData,
	})

	if err != nil {
		log.Printf("Error rendering payments template: %v", err)
		c.String(http.StatusInternalServerError, "Failed to render template: %v", err)
		return
	}

	log.Printf("getPaymentsPageDB: Template rendered successfully")
}

// getPaymentsDataFromDB retrieves payments data from database
func getPaymentsDataFromDB(ctx context.Context, filters PaymentFilters, page, pageSize int) (*PaymentsPageData, error) {
	if database.Queries == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	data := &PaymentsPageData{
		Filters: filters,
		Pagination: PaginationInfo{
			CurrentPage: page,
			PageSize:    pageSize,
		},
	}

	// Build filter parameters
	offset := int32((page - 1) * pageSize)
	limit := int32(pageSize)

	// Build filter parameters - use pointers for nullable fields
	var sitePtr, statusPtr, searchPtr *string
	var startDate, endDate pgtype.Timestamptz

	if filters.Site != "" {
		sitePtr = &filters.Site
	}
	if filters.Status != "" {
		statusPtr = &filters.Status
	}
	if filters.Search != "" {
		searchPtr = &filters.Search
	}
	if filters.StartDate != "" {
		if t, err := time.Parse("2006-01-02", filters.StartDate); err == nil {
			startDate = pgtype.Timestamptz{Time: t, Valid: true}
		}
	}
	if filters.EndDate != "" {
		if t, err := time.Parse("2006-01-02", filters.EndDate); err == nil {
			endDate = pgtype.Timestamptz{Time: t.Add(24 * time.Hour), Valid: true}
		}
	}

	// Get filtered payments grouped by email+address
	payments, err := database.Queries.ListPaymentsGroupedByEmailAddress(ctx, db.ListPaymentsGroupedByEmailAddressParams{
		Site:      sitePtr,
		Status:    statusPtr,
		Search:    searchPtr,
		StartDate: startDate,
		EndDate:   endDate,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch payments: %w", err)
	}

	// Convert to display format
	data.Payments = make([]PaymentDisplay, len(payments))
	for i, p := range payments {
		data.Payments[i] = convertToPaymentDisplayGrouped(p)
	}

	// Get total count for pagination
	totalCount, err := database.Queries.CountPaymentsGroupedByEmailAddress(ctx, db.CountPaymentsGroupedByEmailAddressParams{
		Site:      sitePtr,
		Status:    statusPtr,
		Search:    searchPtr,
		StartDate: startDate,
		EndDate:   endDate,
	})
	if err != nil {
		log.Printf("Error getting payment count: %v", err)
	} else {
		data.Pagination.TotalItems = totalCount
		data.Pagination.TotalPages = int((totalCount + int64(pageSize) - 1) / int64(pageSize))
		data.Pagination.HasNext = page < data.Pagination.TotalPages
		data.Pagination.HasPrev = page > 1
	}

	// Get status distribution
	statusDist, err := database.Queries.GetPaymentStatusDistribution(ctx)
	if err != nil {
		log.Printf("Error getting status distribution: %v", err)
	} else {
		data.StatusCounts = make([]StatusCount, len(statusDist))
		for i, s := range statusDist {
			data.StatusCounts[i] = StatusCount{
				Status:   s.Status,
				Count:    s.Count,
				TotalUSD: interfaceToFloat64(s.TotalUsd),
			}
		}
	}

	// Get site breakdown
	siteBreakdown, err := database.Queries.GetSiteBreakdown(ctx)
	if err != nil {
		log.Printf("Error getting site breakdown: %v", err)
	} else {
		data.SiteBreakdown = make([]SiteStats, len(siteBreakdown))
		for i, s := range siteBreakdown {
			lastPayment := time.Time{}
			if lastPaymentAt, ok := s.LastPaymentAt.(time.Time); ok {
				lastPayment = lastPaymentAt
			}

			data.SiteBreakdown[i] = SiteStats{
				Site:              s.Site,
				TotalPayments:     s.TotalPayments,
				CompletedPayments: s.CompletedPayments,
				PendingPayments:   s.PendingPayments,
				TotalBTC:          interfaceToFloat64(s.TotalBtc),
				TotalUSD:          interfaceToFloat64(s.TotalUsd),
				LastPaymentAt:     lastPayment,
			}
		}
	}

	// Get overall stats based on filters
	// For now, calculate from status counts
	data.Stats = PaymentStats{}
	for _, sc := range data.StatusCounts {
		data.Stats.TotalPayments += sc.Count
		if sc.Status == "completed" {
			data.Stats.CompletedPayments = sc.Count
			data.Stats.TotalUSD = sc.TotalUSD
		} else if sc.Status == "pending" {
			data.Stats.PendingPayments = sc.Count
		} else if sc.Status == "expired" {
			data.Stats.ExpiredPayments = sc.Count
		}
	}

	if data.Stats.TotalPayments > 0 {
		data.Stats.ConversionRate = float64(data.Stats.CompletedPayments) / float64(data.Stats.TotalPayments) * 100
	}

	return data, nil
}

// convertToPaymentDisplayGrouped converts grouped payment query result to display format
func convertToPaymentDisplayGrouped(p db.ListPaymentsGroupedByEmailAddressRow) PaymentDisplay {
	display := PaymentDisplay{
		PaymentID:       p.PaymentID,
		Address:         p.Address,
		Site:            p.Site,
		Currency:        p.Currency,
		Status:          p.Status,
		CreatedAt:       p.PaymentInitiatedAt,
		GenerationCount: p.GenerationCount,
	}

	// Handle confirmations (nullable int32 pointers)
	if p.Confirmations != nil {
		display.Confirmations = *p.Confirmations
	}
	if p.RequiredConfirmations != nil {
		display.RequiredConfs = *p.RequiredConfirmations
	}

	// Convert amounts from pgtype.Numeric
	if amountBTC, err := p.AmountBtc.Float64Value(); err == nil {
		display.AmountBTC = amountBTC.Float64
	}
	if amountUSD, err := p.AmountUsd.Float64Value(); err == nil {
		display.AmountUSD = amountUSD.Float64
	}

	// Handle optional string fields (pointers)
	if p.Email != nil {
		display.Email = *p.Email
	}
	if p.TxHash != nil {
		display.TxHash = *p.TxHash
	}

	// Handle timestamp fields
	if p.ConfirmedAt.Valid {
		display.ConfirmedAt = &p.ConfirmedAt.Time
	}
	if p.CompletedAt.Valid {
		display.CompletedAt = &p.CompletedAt.Time
	}
	if p.ExpiresAt.Valid {
		display.ExpiresAt = &p.ExpiresAt.Time
		if p.Status == "pending" && p.ExpiresAt.Time.After(time.Now()) {
			duration := time.Until(p.ExpiresAt.Time)
			display.TimeUntilExpiry = formatDuration(duration)
		}
	}
	// Handle FirstCreatedAt - it's an interface{} from the query
	if firstCreatedAt, ok := p.FirstCreatedAt.(time.Time); ok {
		display.FirstCreatedAt = &firstCreatedAt
	}

	// Set badge class based on status
	display.StatusBadgeClass = getStatusBadgeClass(p.Status)

	return display
}

// convertToPaymentDisplay converts DB payment to display format
func convertToPaymentDisplay(p db.Payment) PaymentDisplay {
	display := PaymentDisplay{
		PaymentID:     p.PaymentID,
		Address:       p.Address,
		Site:          p.Site,
		Currency:      p.Currency,
		Status:        p.Status,
		CreatedAt:     p.PaymentInitiatedAt,
	}

	// Handle confirmations (nullable int32 pointers)
	if p.Confirmations != nil {
		display.Confirmations = *p.Confirmations
	}
	if p.RequiredConfirmations != nil {
		display.RequiredConfs = *p.RequiredConfirmations
	}

	// Convert amounts from pgtype.Numeric
	if amountBTC, err := p.AmountBtc.Float64Value(); err == nil {
		display.AmountBTC = amountBTC.Float64
	}
	if amountUSD, err := p.AmountUsd.Float64Value(); err == nil {
		display.AmountUSD = amountUSD.Float64
	}

	// Handle optional string fields (pointers)
	if p.Email != nil {
		display.Email = *p.Email
	}
	if p.TxHash != nil {
		display.TxHash = *p.TxHash
	}

	// Handle timestamp fields
	if p.ConfirmedAt.Valid {
		display.ConfirmedAt = &p.ConfirmedAt.Time
	}
	if p.CompletedAt.Valid {
		display.CompletedAt = &p.CompletedAt.Time
	}
	if p.ExpiresAt.Valid {
		display.ExpiresAt = &p.ExpiresAt.Time
		if p.Status == "pending" && p.ExpiresAt.Time.After(time.Now()) {
			duration := time.Until(p.ExpiresAt.Time)
			display.TimeUntilExpiry = formatDuration(duration)
		}
	}

	// Set badge class based on status
	display.StatusBadgeClass = getStatusBadgeClass(p.Status)

	return display
}

// parsePaymentFilters extracts filters from query parameters
func parsePaymentFilters(c *gin.Context) PaymentFilters {
	return PaymentFilters{
		Site:      c.Query("site"),
		Status:    c.Query("status"),
		Search:    c.Query("search"),
		StartDate: c.Query("start_date"),
		EndDate:   c.Query("end_date"),
	}
}

// parseInt safely parses int with default value
func parseInt(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	if val, err := strconv.Atoi(s); err == nil {
		return val
	}
	return defaultVal
}

// getStatusBadgeClass returns CSS class for status badge
func getStatusBadgeClass(status string) string {
	switch status {
	case "completed":
		return "badge-success"
	case "confirmed":
		return "badge-info"
	case "confirming":
		return "badge-warning"
	case "pending":
		return "badge-secondary"
	case "detected":
		return "badge-primary"
	case "expired":
		return "badge-danger"
	case "failed":
		return "badge-danger"
	default:
		return "badge-secondary"
	}
}

// formatDuration formats a duration into human-readable string
func formatDuration(d time.Duration) string {
	if d < 0 {
		return "expired"
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	} else {
		return fmt.Sprintf("%ds", seconds)
	}
}

// GetPaymentsAPIHandler returns payments data as JSON
func GetPaymentsAPIHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	filters := parsePaymentFilters(c)
	page := parseInt(c.Query("page"), 1)
	pageSize := parseInt(c.Query("page_size"), 50)

	paymentsData, err := getPaymentsDataFromDB(ctx, filters, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to load payments data",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, paymentsData)
}

// GetPaymentDetailsAPIHandler returns details for a single payment
func GetPaymentDetailsAPIHandler(c *gin.Context) {
	paymentID := c.Param("id")
	if paymentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Payment ID required"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	payment, err := database.Queries.GetPayment(ctx, paymentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Payment not found"})
		return
	}

	display := convertToPaymentDisplay(payment)
	c.JSON(http.StatusOK, display)
}

// GetPaymentStatsAPIHandler returns payment statistics
func GetPaymentStatsAPIHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get dashboard overview for stats
	overview, err := database.Queries.GetDashboardOverview(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch stats"})
		return
	}

	totalBTC := interfaceToFloat64(overview.TotalBtcReceived)
	totalUSD := interfaceToFloat64(overview.TotalUsdReceived)
	avgBTC := interfaceToFloat64(overview.AvgBtcPerPayment)
	avgUSD := interfaceToFloat64(overview.AvgUsdPerPayment)

	conversionRate := 0.0
	if overview.TotalPayments > 0 {
		conversionRate = float64(overview.CompletedPayments) / float64(overview.TotalPayments) * 100
	}

	stats := PaymentStats{
		TotalPayments:     overview.TotalPayments,
		CompletedPayments: overview.CompletedPayments,
		PendingPayments:   overview.PendingPayments,
		ExpiredPayments:   overview.ExpiredPayments,
		TotalBTC:          totalBTC,
		TotalUSD:          totalUSD,
		AvgBTC:            avgBTC,
		AvgUSD:            avgUSD,
		ConversionRate:    conversionRate,
	}

	c.JSON(http.StatusOK, stats)
}
