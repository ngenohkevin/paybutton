package payment_processing

import (
	"fmt"
	"log"
	"time"
	"github.com/ngenohkevin/paybutton/internals/payments"
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
		var err error

		// Retry logic with exponential backoff
		for attempt := 0; attempt < maxRetries; attempt++ {
			address, err = generateAddressForSite(site, pool.nextIndex)
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
			Address:      address,
			Site:         site,
			Status:       AddressStatusAvailable,
			Index:        pool.nextIndex,
		}

		pool.addresses[address] = pooledAddr
		pool.availableQueue = append(pool.availableQueue, address)

		// Register address-to-site mapping
		RegisterAddressForSite(address, site)

		generated++
		pool.nextIndex++
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

	// NO PRE-GENERATION - Generate on-demand only to avoid gap limit issues
	// Addresses will be generated when users actually request them
	log.Println("Address pools initialized - using on-demand generation only")

	// Start recycling timer
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			log.Println("Running hourly address recycling...")
			RecycleExpiredAddresses()

			// NO PRE-GENERATION - Only recycle expired addresses
			// New addresses are generated on-demand when users request them

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

// generateAddressForSite - Generate address for specific site using existing Blockonomics API
// This maintains compatibility with existing address generation while adding site isolation
func generateAddressForSite(site string, index int) (string, error) {
	config := SiteRegistry[site]

	// Create a site-specific label that includes site name and index
	// This ensures we can track which addresses belong to which site
	// Since we're using Blockonomics with existing xpub, addresses come from same wallet
	// but we maintain separation through labeling and address tracking
	siteLabel := fmt.Sprintf("%s-%s-idx%d", site, config.Name, index)

	// Use existing Blockonomics API but with site-specific labeling
	// NOTE: Addresses still come from same xpub wallet, but we track site association
	address, err := payments.GenerateBitcoinAddress(siteLabel, 0.001)
	if err != nil {
		return "", fmt.Errorf("failed to generate address for site %s at index %d: %v", site, index, err)
	}

	log.Printf("Generated address %s for site %s at index %d (using Blockonomics)", address, site, index)
	return address, nil
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
			"total_addresses":     len(pool.addresses),
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