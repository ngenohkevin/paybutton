package server

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ngenohkevin/paybutton/internals/payment_processing"
)

// getPaymentsPage renders the payments tracking page
func getPaymentsPage(c *gin.Context) {
	session, _ := globalAdminAuth.store.Get(c.Request, "admin-session")
	username, _ := session.Values["username"].(string)

	data := gin.H{
		"Title":      "Payment Tracking",
		"ActivePage": "payments",
		"Username":   username,
	}

	c.Header("Content-Type", "text/html")
	globalAdminAuth.templates["payments"].Execute(c.Writer, data)
}

// getPaymentsAPI returns paginated payment list with filters
func getPaymentsAPI(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	site := c.Query("site")
	status := c.Query("status")
	search := c.Query("search")
	dateRange := c.DefaultQuery("date_range", "all")

	// Create persistence layer
	persistence := payment_processing.NewPaymentPersistence()
	if !persistence.IsEnabled() {
		c.JSON(http.StatusOK, gin.H{
			"payments": []interface{}{},
			"total":    0,
			"page":     page,
			"limit":    limit,
		})
		return
	}

	// Calculate offset
	offset := (page - 1) * limit

	// Parse date range
	var startDate, endDate time.Time
	now := time.Now()
	switch dateRange {
	case "today":
		startDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		endDate = now
	case "week":
		startDate = now.AddDate(0, 0, -7)
		endDate = now
	case "month":
		startDate = now.AddDate(0, -1, 0)
		endDate = now
	case "year":
		startDate = now.AddDate(-1, 0, 0)
		endDate = now
	}

	// Query with filters
	payments, total, err := persistence.ListPaymentsWithFilters(ctx, payment_processing.ListPaymentsParams{
		Site:      site,
		Status:    status,
		Search:    search,
		StartDate: startDate,
		EndDate:   endDate,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"payments": payments,
		"total":    total,
		"page":     page,
		"limit":    limit,
	})
}

// getPaymentStatsAPI returns payment statistics
func getPaymentStatsAPI(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	site := c.DefaultQuery("site", "all")

	persistence := payment_processing.NewPaymentPersistence()
	if !persistence.IsEnabled() {
		c.JSON(http.StatusOK, gin.H{
			"total_payments":  0,
			"completed_count": 0,
			"pending_count":   0,
			"expired_count":   0,
			"total_btc":       0,
			"total_usd":       0,
			"avg_btc":         0,
			"avg_usd":         0,
		})
		return
	}

	stats, err := persistence.GetPaymentStats(ctx, site)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total_payments":  stats.TotalPayments,
		"completed_count": stats.CompletedCount,
		"pending_count":   stats.PendingCount,
		"expired_count":   stats.ExpiredCount,
		"total_btc":       stats.TotalBTC,
		"total_usd":       stats.TotalUSD,
		"avg_btc":         stats.AverageBTC,
		"avg_usd":         stats.AverageUSD,
	})
}

// getPaymentDetailsAPI returns full details for a payment
func getPaymentDetailsAPI(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	paymentID := c.Param("id")

	persistence := payment_processing.NewPaymentPersistence()
	if !persistence.IsEnabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "Payment not found"})
		return
	}

	payment, err := persistence.GetPayment(ctx, paymentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Payment not found"})
		return
	}

	c.JSON(http.StatusOK, payment)
}

// exportPaymentsAPI exports payments to CSV
func exportPaymentsAPI(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get filters
	site := c.Query("site")
	// status := c.Query("status")
	// search := c.Query("search")
	// dateRange := c.Query("date_range")

	persistence := payment_processing.NewPaymentPersistence()
	if !persistence.IsEnabled() {
		c.String(http.StatusInternalServerError, "Payment tracking not enabled")
		return
	}

	// Query payments (using pending for now, will be improved with filtered query)
	payments, err := persistence.ListPendingPayments(ctx)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to fetch payments: "+err.Error())
		return
	}

	// Apply site filter
	var filteredPayments []interface{}
	for _, p := range payments {
		if site == "" || p.Site == site {
			filteredPayments = append(filteredPayments, p)
		}
	}

	// Set CSV headers
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=payments_%s.csv", time.Now().Format("20060102")))

	// Create CSV writer
	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	// Write header
	writer.Write([]string{
		"Payment ID", "Email", "Site", "Address",
		"Amount BTC", "Amount USD", "Currency", "Status",
		"Created At", "Confirmed At", "Completed At", "Expires At",
	})

	// Write payment rows
	for _, payment := range filteredPayments {
		p := payment.(interface{})
		// Convert payment to row (this needs proper type conversion)
		// For now, this is a placeholder that will need adjustment based on actual payment structure
		writer.Write([]string{
			fmt.Sprintf("%v", p),
			// Add proper field extraction here
		})
	}

	writer.Flush()
}
