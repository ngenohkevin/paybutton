package payment_processing

import (
	"context"
	"log"
	"time"

	dbgen "github.com/ngenohkevin/paybutton/internals/db"
	"github.com/ngenohkevin/paybutton/internals/payments"
)

// AddressHealthService performs comprehensive database cleanup and verification
type AddressHealthService struct {
	queries  *dbgen.Queries
	interval time.Duration
	stopChan chan struct{}
}

// NewAddressHealthService creates a new health service
func NewAddressHealthService(queries *dbgen.Queries, interval time.Duration) *AddressHealthService {
	return &AddressHealthService{
		queries:  queries,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

// Start begins the health check service
func (s *AddressHealthService) Start() {
	if s.queries == nil {
		log.Println("⚠️ Health service disabled - no database connection")
		return
	}

	log.Printf("🏥 Address Health Service started (runs every %v)", s.interval)

	// Run immediately on startup
	s.RunHealthCheck()

	// Then run periodically
	ticker := time.NewTicker(s.interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				s.RunHealthCheck()
			case <-s.stopChan:
				ticker.Stop()
				log.Println("🏥 Address Health Service stopped")
				return
			}
		}
	}()
}

// Stop stops the health service
func (s *AddressHealthService) Stop() {
	close(s.stopChan)
}

// RunHealthCheck performs a comprehensive health check and cleanup
func (s *AddressHealthService) RunHealthCheck() {
	log.Println("🏥 ========================================")
	log.Println("🏥 Starting Address Health Check...")
	log.Println("🏥 ========================================")
	startTime := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Get health summary before fixes
	summaryBefore, err := s.queries.HealthCheckSummary(ctx)
	if err != nil {
		log.Printf("⚠️ Could not get health summary: %v", err)
		return
	}

	log.Printf("📊 Current State:")
	log.Printf("   Total addresses: %d", summaryBefore.TotalAddresses)
	log.Printf("   Available: %d, Reserved: %d, Used: %d",
		summaryBefore.AvailableCount, summaryBefore.ReservedCount, summaryBefore.UsedCount)

	issuesFound := summaryBefore.ExpiredReservations +
		summaryBefore.NullPaymentCounts +
		summaryBefore.NullUsedTimestamps +
		summaryBefore.UsedInQueue

	if issuesFound > 0 {
		log.Printf("⚠️  Issues to fix:")
		if summaryBefore.ExpiredReservations > 0 {
			log.Printf("   - Expired reservations (>72h): %d", summaryBefore.ExpiredReservations)
		}
		if summaryBefore.NullPaymentCounts > 0 {
			log.Printf("   - NULL payment counts: %d", summaryBefore.NullPaymentCounts)
		}
		if summaryBefore.NullUsedTimestamps > 0 {
			log.Printf("   - NULL used timestamps: %d", summaryBefore.NullUsedTimestamps)
		}
		if summaryBefore.UsedInQueue > 0 {
			log.Printf("   - Used addresses in queue: %d", summaryBefore.UsedInQueue)
		}
	}

	stats := &HealthStats{}

	// Run all health checks
	s.fixReservedWithPayments(ctx, stats)
	s.removeUsedFromQueue(ctx, stats)
	s.fixNullPaymentCounts(ctx, stats)
	s.verifyExpiredReservations(ctx, stats)
	s.verifyReservedOnBlockchain(ctx, stats)
	s.comprehensiveBlockchainCheck(ctx, stats)

	// Get health summary after fixes
	summaryAfter, errAfter := s.queries.HealthCheckSummary(ctx)

	duration := time.Since(startTime)
	log.Println("🏥 ========================================")
	log.Printf("🏥 Health Check Completed in %v", duration)
	log.Println("🏥 ========================================")
	log.Printf("📊 Results:")
	log.Printf("   Reserved with payments fixed: %d", stats.ReservedWithPaymentFixed)
	log.Printf("   Used removed from queue: %d", stats.UsedRemovedFromQueue)
	log.Printf("   NULL payment counts fixed: %d", stats.NullPaymentCountsFixed)
	log.Printf("   Expired reservations processed: %d", stats.ExpiredReservationsProcessed)
	log.Printf("   Late payments detected: %d", stats.LatePaymentsDetected)
	log.Printf("   Blockchain verifications: %d", stats.BlockchainVerifications)

	totalFixed := stats.TotalIssuesFixed()
	if totalFixed > 0 {
		log.Printf("✅ Fixed %d total issues", totalFixed)
	} else {
		log.Printf("✅ All addresses healthy - no issues found")
	}

	if errAfter == nil {
		log.Printf("📊 Final State:")
		log.Printf("   Available: %d, Reserved: %d, Used: %d",
			summaryAfter.AvailableCount, summaryAfter.ReservedCount, summaryAfter.UsedCount)
	}

	log.Println("🏥 ========================================")
}

// HealthStats tracks health check statistics
type HealthStats struct {
	ReservedWithPaymentFixed     int
	UsedRemovedFromQueue         int
	NullPaymentCountsFixed       int
	ExpiredReservationsProcessed int
	LatePaymentsDetected         int
	BlockchainVerifications      int
}

func (s *HealthStats) TotalIssuesFixed() int {
	return s.ReservedWithPaymentFixed +
		s.UsedRemovedFromQueue +
		s.NullPaymentCountsFixed +
		s.LatePaymentsDetected
}

// fixReservedWithPayments finds and fixes addresses marked as reserved but have completed payments
func (s *AddressHealthService) fixReservedWithPayments(ctx context.Context, stats *HealthStats) {
	log.Println("🔧 Step 1: Checking for reserved addresses with completed payments...")

	// Get count first
	count, err := s.queries.CountReservedWithPayments(ctx)
	if err != nil {
		log.Printf("⚠️ Error counting reserved with payments: %v", err)
		return
	}

	if count == 0 {
		log.Println("   ✅ No reserved addresses with payments found")
		return
	}

	log.Printf("   Found %d reserved addresses with completed payments", count)

	// Get the addresses
	addresses, err := s.queries.FindReservedAddressesWithPayments(ctx)
	if err != nil {
		log.Printf("⚠️ Error finding reserved with payments: %v", err)
		return
	}

	// Fix each one
	for _, addr := range addresses {
		err := s.queries.FixReservedAddressWithPayment(ctx, addr.Address)
		if err != nil {
			log.Printf("   ⚠️ Failed to fix address %s: %v", addr.Address, err)
		} else {
			log.Printf("   ✅ Fixed %s (site: %s, payments: %d)", addr.Address, addr.Site, addr.PaymentCount)
			stats.ReservedWithPaymentFixed++
		}
	}
}

// removeUsedFromQueue removes "used" addresses from the available queue
func (s *AddressHealthService) removeUsedFromQueue(ctx context.Context, stats *HealthStats) {
	log.Println("🔧 Step 2: Removing used addresses from queue...")

	err := s.queries.RemoveUsedAddressesFromQueue(ctx)
	if err != nil {
		log.Printf("⚠️ Error removing used from queue: %v", err)
		return
	}

	// Count how many were removed (exec doesn't return count in sqlc)
	stats.UsedRemovedFromQueue++
	log.Println("   ✅ Cleaned up queue")
}

// fixNullPaymentCounts fixes addresses with NULL payment counts
func (s *AddressHealthService) fixNullPaymentCounts(ctx context.Context, stats *HealthStats) {
	log.Println("🔧 Step 3: Fixing NULL payment counts...")

	err := s.queries.FixNullPaymentCounts(ctx)
	if err != nil {
		log.Printf("⚠️ Error fixing NULL payment counts: %v", err)
		return
	}

	stats.NullPaymentCountsFixed++
	log.Println("   ✅ Fixed NULL payment counts")
}

// verifyExpiredReservations checks expired reservations on blockchain
func (s *AddressHealthService) verifyExpiredReservations(ctx context.Context, stats *HealthStats) {
	log.Println("🔧 Step 4: Verifying expired reservations (>72h) on blockchain...")

	expired, err := s.queries.GetExpiredReservationsForHealthCheck(ctx)
	if err != nil {
		log.Printf("⚠️ Error getting expired reservations: %v", err)
		return
	}

	if len(expired) == 0 {
		log.Println("   ✅ No expired reservations found")
		return
	}

	log.Printf("   Found %d expired reservations to verify", len(expired))

	for _, addr := range expired {
		stats.ExpiredReservationsProcessed++

		// Check on blockchain for late payment
		balance, txCount, err := payments.CheckAddressHistoryWithMempoolSpace(addr.Address)
		if err != nil {
			log.Printf("   ⚠️ Could not verify %s on blockchain: %v", addr.Address, err)
			continue
		}

		if balance > 0 || txCount > 0 {
			// Late payment found!
			log.Printf("   🚨 LATE PAYMENT detected: %s (age: %dh, balance: %d, txs: %d)",
				addr.Address, int(addr.HoursOld), balance, txCount)

			// Mark as used
			err = s.queries.FixReservedAddressWithPayment(ctx, addr.Address)
			if err == nil {
				stats.LatePaymentsDetected++
			}
		} else {
			log.Printf("   ✅ Verified clean: %s (age: %dh)", addr.Address, int(addr.HoursOld))
		}

		// Rate limit to avoid API limits
		time.Sleep(500 * time.Millisecond)
	}
}

// verifyReservedOnBlockchain checks ALL reserved addresses on blockchain
func (s *AddressHealthService) verifyReservedOnBlockchain(ctx context.Context, stats *HealthStats) {
	log.Println("🔧 Step 5: Verifying ALL reserved addresses on blockchain...")

	reserved, err := s.queries.GetAllReservedAddresses(ctx)
	if err != nil {
		log.Printf("⚠️ Error getting reserved addresses: %v", err)
		return
	}

	if len(reserved) == 0 {
		log.Println("   ✅ No reserved addresses to verify")
		return
	}

	log.Printf("   Verifying %d reserved addresses on blockchain...", len(reserved))

	for _, addr := range reserved {
		stats.BlockchainVerifications++

		// Check on blockchain
		balance, txCount, err := payments.CheckAddressHistoryWithMempoolSpace(addr.Address)
		if err != nil {
			log.Printf("   ⚠️ Could not verify %s: %v", addr.Address, err)
			continue
		}

		if balance > 0 || txCount > 0 {
			// Untracked payment found!
			log.Printf("   🚨 UNTRACKED PAYMENT: %s (balance: %d, txs: %d)",
				addr.Address, balance, txCount)

			// Mark as used
			err = s.queries.FixReservedAddressWithPayment(ctx, addr.Address)
			if err == nil {
				stats.LatePaymentsDetected++
			}
		}

		// Rate limit heavily to avoid API bans (1 request per second)
		time.Sleep(1 * time.Second)
	}

	log.Printf("   ✅ Completed %d blockchain verifications", stats.BlockchainVerifications)
}

// comprehensiveBlockchainCheck verifies transaction history of ALL addresses
func (s *AddressHealthService) comprehensiveBlockchainCheck(ctx context.Context, stats *HealthStats) {
	log.Println("🔧 Step 6: COMPREHENSIVE blockchain check of ALL addresses...")

	allAddresses, err := s.queries.GetAllAddressesForBlockchainCheck(ctx)
	if err != nil {
		log.Printf("⚠️ Error getting addresses for blockchain check: %v", err)
		return
	}

	if len(allAddresses) == 0 {
		log.Println("   ✅ No addresses to check")
		return
	}

	log.Printf("   Checking transaction history for %d addresses...", len(allAddresses))
	log.Println("   This may take a while (rate-limited to 1 request/second)")

	inconsistencies := 0
	verified := 0

	for _, addr := range allAddresses {
		verified++

		// Check blockchain for actual transaction history
		balance, txCount, err := payments.CheckAddressHistoryWithMempoolSpace(addr.Address)
		if err != nil {
			log.Printf("   ⚠️ Could not verify %s: %v", addr.Address, err)
			continue
		}

		hasHistory := balance > 0 || txCount > 0

		// Verify consistency between database status and blockchain reality
		switch addr.Status {
		case "reserved":
			if hasHistory {
				// Reserved but has transactions = UNTRACKED PAYMENT
				log.Printf("   🚨 INCONSISTENCY: Address %s is 'reserved' but has %d txs (balance: %d sats)",
					addr.Address, txCount, balance)
				log.Printf("      Site: %s, Email: %s", addr.Site, stringOrEmpty(addr.Email))

				// Fix it
				err = s.queries.FixReservedAddressWithPayment(ctx, addr.Address)
				if err == nil {
					stats.LatePaymentsDetected++
					inconsistencies++
				}
			}

		case "used":
			if !hasHistory {
				// Marked as used but has NO transactions = INCORRECT STATUS
				log.Printf("   🚨 INCONSISTENCY: Address %s is 'used' but has NO transactions on blockchain",
					addr.Address)
				log.Printf("      Site: %s, Payment count: %v", addr.Site, intPtrToString(addr.PaymentCount))
				inconsistencies++
			} else {
				// Verify payment count matches transaction count
				dbPaymentCount := 0
				if addr.PaymentCount != nil {
					dbPaymentCount = int(*addr.PaymentCount)
				}

				if int(txCount) != dbPaymentCount && dbPaymentCount > 0 {
					log.Printf("   ⚠️ Payment count mismatch: %s has %d txs on blockchain but DB shows %d",
						addr.Address, txCount, dbPaymentCount)
				}
			}

		case "available":
			if hasHistory {
				// Available but has transactions = SHOULD BE MARKED USED OR SKIPPED
				log.Printf("   🚨 INCONSISTENCY: Address %s is 'available' but has %d txs (balance: %d sats)",
					addr.Address, txCount, balance)
				log.Printf("      Site: %s - This address should be marked as 'used' or 'skipped'", addr.Site)
				inconsistencies++

				// Mark as used
				err = s.queries.FixReservedAddressWithPayment(ctx, addr.Address)
				if err == nil {
					stats.LatePaymentsDetected++
				}
			}
		}

		// Rate limit: 1 request per second to avoid API bans
		time.Sleep(1 * time.Second)

		// Progress indicator every 5 addresses
		if verified%5 == 0 {
			log.Printf("   Progress: %d/%d addresses checked...", verified, len(allAddresses))
		}
	}

	log.Printf("   ✅ Comprehensive check complete: %d addresses verified, %d inconsistencies found",
		verified, inconsistencies)

	if inconsistencies == 0 {
		log.Println("   ✅ All addresses consistent with blockchain state!")
	}
}

// Helper functions
func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func intPtrToString(i *int32) string {
	if i == nil {
		return "NULL"
	}
	return string(rune(*i))
}
