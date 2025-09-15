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

	// Generate addresses sequentially to avoid ANY gaps
	for i := 0; i < count && pool.nextIndex <= config.EndIndex; i++ {
		// Generate address using site's derivation path and index
		address, err := generateAddressForSite(site, pool.nextIndex)
		if err != nil {
			log.Printf("Error generating address at index %d for %s: %v",
				pool.nextIndex, site, err)
			return err
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

	log.Printf("Pre-generated %d addresses for site %s (indices %d-%d, pool size: %d)",
		generated, site, pool.nextIndex-generated, pool.nextIndex-1, len(pool.availableQueue))

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

	// Pre-generate 100 addresses per site to start
	for site := range SiteRegistry {
		if err := PreGenerateAddressPool(site, 100); err != nil {
			log.Printf("Error initializing pool for %s: %v", site, err)
		}
	}

	// Start recycling timer
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			log.Println("Running hourly address recycling...")
			RecycleExpiredAddresses()

			// Top up pools if running low
			for site := range SiteRegistry {
				pool := GetSitePool(site)
				pool.mutex.RLock()
				availableCount := len(pool.availableQueue)
				pool.mutex.RUnlock()

				if availableCount < 20 {
					log.Printf("Pool for %s running low (%d available), generating more...",
						site, availableCount)
					PreGenerateAddressPool(site, 50)
				}

				// Check gap limit
				consecutiveUnpaid, isAtRisk := pool.GetGapLimitStatus()
				if isAtRisk {
					log.Printf("GAP LIMIT WARNING: Site %s has %d consecutive unpaid addresses!",
						site, consecutiveUnpaid)
				}
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