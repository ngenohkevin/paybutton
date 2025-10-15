package payment_processing

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ngenohkevin/paybutton/internals/database"
	dbgen "github.com/ngenohkevin/paybutton/internals/db"
)

// PaymentPersistence handles database operations for payment tracking
type PaymentPersistence struct {
	queries *dbgen.Queries
	enabled bool
}

// NewPaymentPersistence creates a new payment persistence layer
func NewPaymentPersistence() *PaymentPersistence {
	if !database.IsEnabled() {
		log.Println("⚠️ Payment persistence disabled - running in memory-only mode")
		return &PaymentPersistence{enabled: false}
	}

	if database.Queries == nil {
		log.Println("⚠️ Database not initialized - payment persistence disabled")
		return &PaymentPersistence{enabled: false}
	}

	return &PaymentPersistence{
		queries: database.Queries,
		enabled: true,
	}
}

// IsEnabled returns whether persistence is active
func (p *PaymentPersistence) IsEnabled() bool {
	return p.enabled
}

// CreatePayment creates a new payment record
func (p *PaymentPersistence) CreatePayment(ctx context.Context, params PaymentParams) (*dbgen.Payment, error) {
	if !p.enabled {
		return nil, nil
	}

	// Convert amounts to pgtype.Numeric
	amountBtc := pgtype.Numeric{}
	if err := amountBtc.Scan(fmt.Sprintf("%.8f", params.AmountBTC)); err != nil {
		return nil, fmt.Errorf("failed to convert BTC amount: %w", err)
	}

	amountUsd := pgtype.Numeric{}
	if params.AmountUSD > 0 {
		if err := amountUsd.Scan(fmt.Sprintf("%.2f", params.AmountUSD)); err != nil {
			return nil, fmt.Errorf("failed to convert USD amount: %w", err)
		}
	}

	// Convert expires_at
	expiresAt := pgtype.Timestamptz{}
	if !params.ExpiresAt.IsZero() {
		expiresAt = pgtype.Timestamptz{
			Time:  params.ExpiresAt,
			Valid: true,
		}
	}

	// Convert required confirmations
	var requiredConfs *int32
	if params.RequiredConfirmations > 0 {
		val := int32(params.RequiredConfirmations)
		requiredConfs = &val
	}

	payment, err := p.queries.CreatePayment(ctx, dbgen.CreatePaymentParams{
		PaymentID:             params.PaymentID,
		Address:               params.Address,
		Site:                  params.Site,
		AmountBtc:             amountBtc,
		AmountUsd:             amountUsd,
		Currency:              params.Currency,
		Email:                 &params.Email,
		OrderID:               &params.OrderID,
		UserAgent:             &params.UserAgent,
		IpAddress:             &params.IPAddress,
		RequiredConfirmations: requiredConfs,
		ExpiresAt:             expiresAt,
	})

	if err != nil {
		log.Printf("❌ Failed to create payment %s: %v", params.PaymentID, err)
		return nil, err
	}

	log.Printf("✅ Created payment record %s for %s (%.8f BTC)", params.PaymentID, params.Email, params.AmountBTC)
	return &payment, nil
}

// UpdatePaymentTransaction updates payment with transaction details
func (p *PaymentPersistence) UpdatePaymentTransaction(ctx context.Context, paymentID, txHash string, confirmations int) error {
	if !p.enabled {
		return nil
	}

	confs := int32(confirmations)
	err := p.queries.UpdatePaymentTransaction(ctx, dbgen.UpdatePaymentTransactionParams{
		PaymentID:     paymentID,
		TxHash:        &txHash,
		Status:        "detected",
		Confirmations: &confs,
	})

	if err != nil {
		log.Printf("❌ Failed to update payment transaction %s: %v", paymentID, err)
		return err
	}

	log.Printf("✅ Updated payment %s with tx %s (%d confirmations)", paymentID, txHash, confirmations)
	return nil
}

// UpdatePaymentConfirmed marks payment as confirmed
func (p *PaymentPersistence) UpdatePaymentConfirmed(ctx context.Context, paymentID string, confirmations int) error {
	if !p.enabled {
		return nil
	}

	confs := int32(confirmations)
	err := p.queries.UpdatePaymentConfirmed(ctx, dbgen.UpdatePaymentConfirmedParams{
		PaymentID:     paymentID,
		Confirmations: &confs,
	})

	if err != nil {
		log.Printf("❌ Failed to mark payment confirmed %s: %v", paymentID, err)
		return err
	}

	log.Printf("✅ Payment %s confirmed with %d confirmations", paymentID, confirmations)
	return nil
}

// UpdatePaymentCompleted marks payment as completed
func (p *PaymentPersistence) UpdatePaymentCompleted(ctx context.Context, paymentID string) error {
	if !p.enabled {
		return nil
	}

	err := p.queries.UpdatePaymentCompleted(ctx, paymentID)
	if err != nil {
		log.Printf("❌ Failed to mark payment completed %s: %v", paymentID, err)
		return err
	}

	log.Printf("✅ Payment %s marked as completed", paymentID)
	return nil
}

// MarkPaymentExpired marks payment as expired
func (p *PaymentPersistence) MarkPaymentExpired(ctx context.Context, paymentID string) error {
	if !p.enabled {
		return nil
	}

	err := p.queries.MarkPaymentExpired(ctx, paymentID)
	if err != nil {
		log.Printf("❌ Failed to mark payment expired %s: %v", paymentID, err)
		return err
	}

	log.Printf("⏰ Payment %s marked as expired", paymentID)
	return nil
}

// MarkWebhookSent marks webhook as sent
func (p *PaymentPersistence) MarkWebhookSent(ctx context.Context, paymentID string) error {
	if !p.enabled {
		return nil
	}

	return p.queries.UpdatePaymentWebhookSent(ctx, paymentID)
}

// MarkEmailSent marks email as sent
func (p *PaymentPersistence) MarkEmailSent(ctx context.Context, paymentID string) error {
	if !p.enabled {
		return nil
	}

	return p.queries.UpdatePaymentEmailSent(ctx, paymentID)
}

// MarkTelegramSent marks telegram notification as sent
func (p *PaymentPersistence) MarkTelegramSent(ctx context.Context, paymentID string) error {
	if !p.enabled {
		return nil
	}

	return p.queries.UpdatePaymentTelegramSent(ctx, paymentID)
}

// GetPayment retrieves a payment by ID
func (p *PaymentPersistence) GetPayment(ctx context.Context, paymentID string) (*dbgen.Payment, error) {
	if !p.enabled {
		return nil, nil
	}

	payment, err := p.queries.GetPayment(ctx, paymentID)
	if err != nil {
		return nil, err
	}

	return &payment, nil
}

// GetPaymentByAddress retrieves the most recent active payment for an address
func (p *PaymentPersistence) GetPaymentByAddress(ctx context.Context, address string) (*dbgen.Payment, error) {
	if !p.enabled {
		return nil, nil
	}

	payment, err := p.queries.GetPaymentByAddress(ctx, address)
	if err != nil {
		return nil, err
	}

	return &payment, nil
}

// ListPendingPayments retrieves all pending payments
func (p *PaymentPersistence) ListPendingPayments(ctx context.Context) ([]dbgen.Payment, error) {
	if !p.enabled {
		return nil, nil
	}

	return p.queries.ListPendingPayments(ctx)
}

// ListExpiredPayments retrieves payments that have expired
func (p *PaymentPersistence) ListExpiredPayments(ctx context.Context) ([]dbgen.Payment, error) {
	if !p.enabled {
		return nil, nil
	}

	return p.queries.ListExpiredPayments(ctx)
}

// GetPaymentStats retrieves payment statistics for a site
func (p *PaymentPersistence) GetPaymentStats(ctx context.Context, site string) (*PaymentStats, error) {
	if !p.enabled {
		return nil, nil
	}

	stats, err := p.queries.GetPaymentStats(ctx, site)
	if err != nil {
		return nil, err
	}

	// Convert the interface{} values to pgtype.Numeric for numericToFloat
	totalBtc, _ := stats.TotalBtc.(pgtype.Numeric)
	totalUsd, _ := stats.TotalUsd.(pgtype.Numeric)
	avgBtc, _ := stats.AvgBtc.(pgtype.Numeric)
	avgUsd, _ := stats.AvgUsd.(pgtype.Numeric)

	return &PaymentStats{
		TotalPayments:  stats.TotalPayments,
		CompletedCount: stats.CompletedCount,
		PendingCount:   stats.PendingCount,
		ExpiredCount:   stats.ExpiredCount,
		TotalBTC:       numericToFloat(totalBtc),
		TotalUSD:       numericToFloat(totalUsd),
		AverageBTC:     numericToFloat(avgBtc),
		AverageUSD:     numericToFloat(avgUsd),
	}, nil
}

// ListPaymentsWithFilters retrieves payments with filters and pagination
func (p *PaymentPersistence) ListPaymentsWithFilters(ctx context.Context, params ListPaymentsParams) ([]dbgen.Payment, int64, error) {
	if !p.enabled {
		return nil, 0, nil
	}

	// Prepare nullable parameters for sqlc
	var siteParam *string
	if params.Site != "" {
		siteParam = &params.Site
	}

	var statusParam *string
	if params.Status != "" {
		statusParam = &params.Status
	}

	var searchParam *string
	if params.Search != "" {
		searchParam = &params.Search
	}

	var startDateParam pgtype.Timestamptz
	if !params.StartDate.IsZero() {
		startDateParam = pgtype.Timestamptz{
			Time:  params.StartDate,
			Valid: true,
		}
	}

	var endDateParam pgtype.Timestamptz
	if !params.EndDate.IsZero() {
		endDateParam = pgtype.Timestamptz{
			Time:  params.EndDate,
			Valid: true,
		}
	}

	// Get total count
	count, err := p.queries.CountPaymentsWithFilters(ctx, dbgen.CountPaymentsWithFiltersParams{
		Site:      siteParam,
		Status:    statusParam,
		Search:    searchParam,
		StartDate: startDateParam,
		EndDate:   endDateParam,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count payments: %w", err)
	}

	// Get paginated results
	payments, err := p.queries.ListPaymentsWithFilters(ctx, dbgen.ListPaymentsWithFiltersParams{
		Site:      siteParam,
		Status:    statusParam,
		Search:    searchParam,
		StartDate: startDateParam,
		EndDate:   endDateParam,
		Limit:     int32(params.Limit),
		Offset:    int32(params.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list payments: %w", err)
	}

	return payments, count, nil
}

// PaymentParams holds parameters for creating a payment
type PaymentParams struct {
	PaymentID              string
	Address                string
	Site                   string
	AmountBTC              float64
	AmountUSD              float64
	Currency               string
	Email                  string
	OrderID                string
	UserAgent              string
	IPAddress              string
	RequiredConfirmations  int
	ExpiresAt              time.Time
}

// PaymentStats holds payment statistics
type PaymentStats struct {
	TotalPayments  int64
	CompletedCount int64
	PendingCount   int64
	ExpiredCount   int64
	TotalBTC       float64
	TotalUSD       float64
	AverageBTC     float64
	AverageUSD     float64
}

// ListPaymentsParams holds parameters for filtering and paginating payments
type ListPaymentsParams struct {
	Site      string
	Status    string
	Search    string
	StartDate time.Time
	EndDate   time.Time
	Limit     int
	Offset    int
}

// Helper function to convert pgtype.Numeric to float64
func numericToFloat(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}

	// Convert to string first, then parse as float
	var result float64
	fmt.Sscanf(n.Int.String(), "%f", &result)

	// Apply the exponent
	exp := n.Exp
	for exp > 0 {
		result *= 10
		exp--
	}
	for exp < 0 {
		result /= 10
		exp++
	}

	return result
}

// StartPaymentCleanupJob runs a background job that marks expired payments
func StartPaymentCleanupJob() {
	log.Println("🧹 Starting payment cleanup job (runs every 10 minutes)")
	
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	// Run immediately on startup
	cleanupExpiredPayments()

	// Then run every 10 minutes
	for range ticker.C {
		cleanupExpiredPayments()
	}
}

// cleanupExpiredPayments marks all expired pending payments as expired
func cleanupExpiredPayments() {
	persistence := NewPaymentPersistence()
	if !persistence.IsEnabled() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get all expired payments
	expiredPayments, err := persistence.ListExpiredPayments(ctx)
	if err != nil {
		log.Printf("❌ Failed to list expired payments: %v", err)
		return
	}

	if len(expiredPayments) == 0 {
		return
	}

	log.Printf("🧹 Found %d expired payments to clean up", len(expiredPayments))

	// Mark each as expired
	for _, payment := range expiredPayments {
		err := persistence.MarkPaymentExpired(ctx, payment.PaymentID)
		if err != nil {
			log.Printf("❌ Failed to mark payment %s as expired: %v", payment.PaymentID, err)
			continue
		}
		log.Printf("⏰ Marked payment %s as expired (address: %s, email: %s)",
			payment.PaymentID, payment.Address, payment.Email)
	}

	log.Printf("✅ Cleanup complete: marked %d payments as expired", len(expiredPayments))
}
