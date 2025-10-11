package payment_processing

import (
	"fmt"
	"github.com/ngenohkevin/paybutton/internals/payments"
	"log"
	"time"
)

// PreGenerateAddressPool - Generate addresses in bulk to avoid gaps
func PreGenerateAddressPool(site string, count int) error {
	config := SiteRegistry[site]
	pool := GetSitePool(site)

	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	generated := 0
	maxRetries := 3
	retryDelay := time.Second

	// Generate addresses sequentially to avoid ANY gaps
	for i := 0; i < count && pool.nextIndex <= config.EndIndex; i++ {
		var address string
		var actualIndex int
		var err error

		// Retry logic with exponential backoff
		for attempt := 0; attempt < maxRetries; attempt++ {
			address, actualIndex, err = generateAddressForSite(site, pool.nextIndex)
			if err == nil {
				break // Success
			}

			if attempt < maxRetries-1 {
				log.Printf("Retry %d/%d for address at index %d for %s after error: %v",
					attempt+1, maxRetries, pool.nextIndex, site, err)
				time.Sleep(retryDelay * time.Duration(attempt+1))
			}
		}

		if err != nil {
			// After all retries failed, stop here to avoid creating gaps
			log.Printf("Failed to generate address at index %d for %s after %d attempts: %v",
				pool.nextIndex, site, maxRetries, err)
			if generated > 0 {
				log.Printf("Partially generated %d addresses for %s before failure", generated, site)
			}
			return fmt.Errorf("address generation failed at index %d: %w", pool.nextIndex, err)
		}

		// Add to pool
		pooledAddr := &PooledAddress{
			Address: address,
			Site:    site,
			Status:  AddressStatusAvailable,
			Index:   actualIndex,
		}

		pool.addresses[address] = pooledAddr
		pool.availableQueue = append(pool.availableQueue, address)

		// Register address-to-site mapping
		RegisterAddressForSite(address, site)

		generated++
		// Update nextIndex to the next index after the one we actually used
		pool.nextIndex = actualIndex + 1
	}

	if generated > 0 {
		log.Printf("Pre-generated %d addresses for site %s (indices %d-%d, pool size: %d)",
			generated, site, pool.nextIndex-generated, pool.nextIndex-1, len(pool.availableQueue))
	}

	// Check gap limit status
	consecutiveUnpaid, isAtRisk := pool.GetGapLimitStatus()
	if isAtRisk {
		log.Printf("WARNING: Site %s approaching gap limit! %d consecutive unpaid addresses",
			site, consecutiveUnpaid)
	}

	return nil
}

// InitializeAddressPools - Run on startup
func InitializeAddressPools() {
	log.Println("Initializing address pools for all sites...")

	// PRE-GENERATE addresses with history checks in background
	// This provides instant address assignment while ensuring clean addresses
	log.Println("Starting background address pre-generation with history verification...")

	// Start background pool maintainer for each site
	for siteName := range SiteRegistry {
		site := siteName // Capture for goroutine
		go maintainSiteAddressPool(site)
	}

	// Start recycling timer
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			log.Println("Running hourly address recycling...")
			RecycleExpiredAddresses()

			// Check gap limit status for monitoring
			for site := range SiteRegistry {
				pool := GetSitePool(site)
				consecutiveUnpaid, isAtRisk := pool.GetGapLimitStatus()
				if isAtRisk {
					log.Printf("GAP LIMIT WARNING: Site %s has %d consecutive unpaid addresses!",
						site, consecutiveUnpaid)
				}

				pool.mutex.RLock()
				totalAddresses := len(pool.addresses)
				availableCount := len(pool.availableQueue)
				pool.mutex.RUnlock()

				log.Printf("Site %s status: %d total addresses, %d available for reuse",
					site, totalAddresses, availableCount)
			}
		}
	}()

	log.Println("Address pool initialization complete")
}

// maintainSiteAddressPool - Background goroutine that keeps pool filled with verified addresses
func maintainSiteAddressPool(site string) {
	minPoolSize := 5  // Minimum verified addresses to keep ready
	maxPoolSize := 10 // Maximum to pre-generate
	refillThreshold := 3 // Refill when pool drops to this size

	// Initial generation
	log.Printf("Pre-generating initial address pool for site %s...", site)
	if err := refillSitePool(site, minPoolSize, maxPoolSize); err != nil {
		log.Printf("Warning: Initial pool generation for %s failed: %v", site, err)
	}

	// Monitor and refill every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		pool := GetSitePool(site)
		pool.mutex.RLock()
		availableCount := len(pool.availableQueue)
		pool.mutex.RUnlock()

		if availableCount <= refillThreshold {
			log.Printf("Pool for %s running low (%d addresses), refilling...", site, availableCount)
			if err := refillSitePool(site, minPoolSize, maxPoolSize); err != nil {
				log.Printf("Warning: Pool refill for %s failed: %v", site, err)
			}
		}
	}
}

// refillSitePool - Generates verified addresses to fill the pool
func refillSitePool(site string, minSize, maxSize int) error {
	pool := GetSitePool(site)

	pool.mutex.RLock()
	currentSize := len(pool.availableQueue)
	nextIndex := pool.nextIndex
	pool.mutex.RUnlock()

	// Calculate how many we need
	needed := minSize - currentSize
	if needed <= 0 {
		return nil // Pool already has enough
	}

	// Don't exceed max size
	if currentSize + needed > maxSize {
		needed = maxSize - currentSize
	}

	if needed <= 0 {
		return nil
	}

	log.Printf("Generating %d verified addresses for site %s (current: %d, target: %d)...",
		needed, site, currentSize, minSize)

	generated := 0
	failures := 0
	maxFailures := 5

	for generated < needed && failures < maxFailures {
		// Generate with history check
		address, actualIndex, err := generateAddressForSite(site, nextIndex)
		if err != nil {
			log.Printf("Failed to generate address for %s at index %d: %v", site, nextIndex, err)
			failures++
			nextIndex++ // Try next index even on failure
			continue
		}

		// Add to pool
		pool.mutex.Lock()

		// Double-check it's not already in pool
		if _, exists := pool.addresses[address]; exists {
			log.Printf("Address %s already in pool for %s, skipping", address, site)
			pool.mutex.Unlock()
			nextIndex = actualIndex + 1
			continue
		}

		pooledAddr := &PooledAddress{
			Address:     address,
			Site:        site,
			Status:      AddressStatusAvailable,
			Index:       actualIndex,
			LastChecked: time.Now(),
		}

		pool.addresses[address] = pooledAddr
		pool.availableQueue = append(pool.availableQueue, address)
		pool.nextIndex = actualIndex + 1

		pool.mutex.Unlock()

		// Register address-to-site mapping
		RegisterAddressForSite(address, site)

		generated++
		nextIndex = actualIndex + 1

		log.Printf("✅ Added verified address %s to pool for %s (pool size: %d/%d)",
			address, site, currentSize+generated, minSize)

		// Small delay to avoid rate limiting mempool.space
		time.Sleep(1 * time.Second)
	}

	if generated > 0 {
		log.Printf("Successfully generated %d verified addresses for site %s", generated, site)
	}

	if failures >= maxFailures {
		return fmt.Errorf("too many failures generating addresses for site %s", site)
	}

	return nil
}

// generateAddressForSite - Generate address for specific site using existing Blockonomics API
// This maintains compatibility with existing address generation while adding site isolation
// CRITICAL: Checks for address history to prevent reuse of addresses with transactions
// Returns: (address, actualIndexUsed, error)
func generateAddressForSite(site string, index int) (string, int, error) {
	config := SiteRegistry[site]
	maxAttempts := 10 // Try up to 10 indices if we hit used addresses

	for attempt := 0; attempt < maxAttempts; attempt++ {
		currentIndex := index + attempt

		// Check if we've exceeded the end index for this site
		if currentIndex > config.EndIndex {
			return "", -1, fmt.Errorf("address index limit reached for site %s (tried indices %d-%d)", site, index, currentIndex)
		}

		// Create a site-specific label that includes site name and index
		// This ensures we can track which addresses belong to which site
		// Since we're using Blockonomics with existing xpub, addresses come from same wallet
		// but we maintain separation through labeling and address tracking
		siteLabel := fmt.Sprintf("%s-%s-idx%d", site, config.Name, currentIndex)

		// Use existing Blockonomics API but with site-specific labeling
		// NOTE: Addresses still come from same xpub wallet, but we track site association
		address, err := payments.GenerateBitcoinAddress(siteLabel, 0.001)
		if err != nil {
			return "", -1, fmt.Errorf("failed to generate address for site %s at index %d: %v", site, currentIndex, err)
		}

		// CRITICAL: Check if this address has transaction history using mempool.space
		// This prevents reuse of addresses from xpub collision or gap limit recycling
		balance, txCount, err := payments.CheckAddressHistoryWithMempoolSpace(address)
		if err != nil {
			// If we can't check, log warning but continue (don't block on API failures)
			log.Printf("⚠️ WARNING: Could not verify address %s history at index %d: %v", address, currentIndex, err)
			log.Printf("⚠️ Proceeding with address generation (mempool.space check failed)")
			log.Printf("Generated address %s for site %s at index %d (using Blockonomics)", address, site, currentIndex)
			return address, currentIndex, nil
		}

		// Check if address has ANY history (balance OR transactions)
		if balance > 0 || txCount > 0 {
			log.Printf("🚨 CRITICAL: Address %s at index %d has HISTORY (balance: %d sats, txs: %d) - SKIPPING", address, currentIndex, balance, txCount)
			log.Printf("🚨 Trying next index %d for site %s", currentIndex+1, site)
			continue // Try next index
		}

		// Success - clean address with no history
		log.Printf("✅ Generated CLEAN address %s for site %s at index %d (0 balance, 0 txs)", address, site, currentIndex)
		return address, currentIndex, nil
	}

	return "", -1, fmt.Errorf("failed to find clean address for site %s after trying %d indices (starting from %d)", site, maxAttempts, index)
}

// GetPoolStats - Get statistics for monitoring
func GetPoolStats() map[string]interface{} {
	stats := make(map[string]interface{})

	for siteName, pool := range sitePools {
		pool.mutex.RLock()

		// Count addresses by status
		availableCount := len(pool.availableQueue)
		reservedCount := 0
		usedCount := 0

		for _, addr := range pool.addresses {
			switch addr.Status {
			case AddressStatusReserved:
				reservedCount++
			case AddressStatusUsed:
				usedCount++
			}
		}

		consecutiveUnpaid, isAtRisk := pool.GetGapLimitStatus()

		stats[siteName] = map[string]interface{}{
			"total_addresses":    len(pool.addresses),
			"available":          availableCount,
			"reserved":           reservedCount,
			"used":               usedCount,
			"next_index":         pool.nextIndex,
			"consecutive_unpaid": consecutiveUnpaid,
			"gap_limit_risk":     isAtRisk,
		}

		pool.mutex.RUnlock()
	}

	return stats
}
