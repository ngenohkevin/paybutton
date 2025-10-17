package payment_processing

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ngenohkevin/paybutton/internals/payments"
)

type AddressStatus string

const (
	AddressStatusAvailable AddressStatus = "available" // Can be assigned
	AddressStatusReserved  AddressStatus = "reserved"  // Assigned, waiting for payment
	AddressStatusUsed      AddressStatus = "used"      // Payment received
	AddressStatusExpired   AddressStatus = "expired"   // 72h passed, ready to recycle
	AddressStatusSkipped   AddressStatus = "skipped"   // Has history, permanently skipped
)

type PooledAddress struct {
	Address      string
	Site         string
	Email        string // Current or last user
	Status       AddressStatus
	ReservedAt   time.Time // When it was assigned
	LastChecked  time.Time // Last balance check
	PaymentCount int       // How many times it received payment
	Index        int       // Address index in HD wallet
}

type SiteAddressPool struct {
	site           string
	addresses      map[string]*PooledAddress // address -> details
	emailToAddress map[string]string         // email -> current address
	availableQueue []string                  // FIFO queue of available addresses
	nextIndex      int                       // Next index to generate
	mutex          sync.RWMutex
	persistence    *PoolPersistence // Database persistence layer
}

var (
	sitePools      = make(map[string]*SiteAddressPool)
	poolMutex      sync.RWMutex
	globalPool     = &GlobalAddressPool{
		availableAddresses: make([]string, 0),
		assignedToSite:     make(map[string]string), // address -> site
	}
	globalPoolMutex sync.RWMutex
)

// GlobalAddressPool - Shared pool of addresses available to any site
type GlobalAddressPool struct {
	availableAddresses []string          // FIFO queue of addresses available to ANY site
	assignedToSite     map[string]string // address -> currently assigned site
}

// AddToGlobalPool - Add address to shared global pool
func AddToGlobalPool(address string) {
	globalPoolMutex.Lock()
	defer globalPoolMutex.Unlock()

	// Only add if not already in global pool
	for _, addr := range globalPool.availableAddresses {
		if addr == address {
			return
		}
	}

	globalPool.availableAddresses = append(globalPool.availableAddresses, address)
	log.Printf("➕ Added address to GLOBAL pool (total available: %d)", len(globalPool.availableAddresses))
}

// TakeFromGlobalPool - Take address from global pool and assign to site
func TakeFromGlobalPool(site string) (string, bool) {
	globalPoolMutex.Lock()
	defer globalPoolMutex.Unlock()

	if len(globalPool.availableAddresses) == 0 {
		return "", false
	}

	// Take first available address
	address := globalPool.availableAddresses[0]
	globalPool.availableAddresses = globalPool.availableAddresses[1:]
	globalPool.assignedToSite[address] = site

	log.Printf("🌍 GLOBAL POOL: Assigned address to site %s (remaining: %d)", site, len(globalPool.availableAddresses))
	return address, true
}

// ReturnToGlobalPool - Return address to global pool when recycled
func ReturnToGlobalPool(address string) {
	globalPoolMutex.Lock()
	defer globalPoolMutex.Unlock()

	// Remove site assignment
	delete(globalPool.assignedToSite, address)

	// Add back to available pool
	globalPool.availableAddresses = append(globalPool.availableAddresses, address)
	log.Printf("♻️ Returned address to GLOBAL pool (total available: %d)", len(globalPool.availableAddresses))
}

// GetGlobalPoolStats - Get global pool statistics
func GetGlobalPoolStats() map[string]interface{} {
	globalPoolMutex.RLock()
	defer globalPoolMutex.RUnlock()

	return map[string]interface{}{
		"available": len(globalPool.availableAddresses),
		"assigned":  len(globalPool.assignedToSite),
	}
}

func GetSitePool(site string) *SiteAddressPool {
	poolMutex.RLock()
	pool, exists := sitePools[site]
	poolMutex.RUnlock()

	if !exists {
		poolMutex.Lock()
		config := SiteRegistry[site]
		pool = &SiteAddressPool{
			site:           site,
			addresses:      make(map[string]*PooledAddress),
			emailToAddress: make(map[string]string),
			availableQueue: make([]string, 0),
			nextIndex:      config.StartIndex,
		}
		sitePools[site] = pool
		poolMutex.Unlock()
	}
	return pool
}

// LoadFromDatabase loads pool state from database
func (p *SiteAddressPool) LoadFromDatabase() error {
	if p.persistence == nil || !p.persistence.IsEnabled() {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Load nextIndex from database
	nextIndex, err := p.persistence.LoadPoolState(ctx, p.site)
	if err != nil {
		return fmt.Errorf("failed to load pool state: %w", err)
	}
	p.nextIndex = nextIndex
	log.Printf("✅ Loaded nextIndex=%d for site %s from database", nextIndex, p.site)

	// Load all addresses from database
	addresses, err := p.persistence.LoadAllAddresses(ctx, p.site)
	if err != nil {
		return fmt.Errorf("failed to load addresses: %w", err)
	}

	for _, addr := range addresses {
		p.addresses[addr.Address] = addr
		if addr.Email != "" && addr.Status == AddressStatusReserved {
			p.emailToAddress[addr.Email] = addr.Address
		}
	}

	// Load queue from database
	queue, err := p.persistence.GetAvailableQueue(ctx, p.site)
	if err != nil {
		return fmt.Errorf("failed to load queue: %w", err)
	}
	p.availableQueue = queue

	// Add available addresses to GLOBAL pool for cross-site reuse
	for _, addr := range queue {
		AddToGlobalPool(addr)
	}

	log.Printf("✅ Loaded %d addresses, %d in queue for site %s (added to global pool)",
		len(addresses), len(queue), p.site)

	return nil
}

// GetOrReuseAddress - Primary function for address assignment with aggressive reuse
func (p *SiteAddressPool) GetOrReuseAddress(email string, amount float64) (string, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// PRIORITY 1: Check if user already has an unpaid address for this site
	if existingAddr, exists := p.emailToAddress[email]; exists {
		if addr := p.addresses[existingAddr]; addr != nil {
			if addr.Status == AddressStatusReserved {
				// ALWAYS REUSE same address for same user - GAP LIMIT PREVENTION
				// Edge case (2x per month): User pays after monitoring stops, then requests new product
				// Solution: Website should show timeout warning to user
				now := time.Now()
				addressAge := now.Sub(addr.ReservedAt)
				addr.LastChecked = now

				log.Printf("REUSING unpaid address %s for %s on %s (reserved %v ago) - GAP LIMIT PREVENTION",
					existingAddr, email, p.site, addressAge.Round(time.Minute))

				// Save to database
				if p.persistence != nil && p.persistence.IsEnabled() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_ = p.persistence.SaveAddress(ctx, addr)
				}

				return existingAddr, nil
			}
		}
	}

	// PRIORITY 2: Find any expired address (72h old, unpaid) to recycle
	// Note: RecycleExpiredAddresses() background job already verifies these for late payments
	now := time.Now()
	for address, addr := range p.addresses {
		if addr.Status == AddressStatusReserved &&
			now.Sub(addr.ReservedAt) > 72*time.Hour {
			// Recycle this address to prevent gap limit!
			log.Printf("RECYCLING expired address %s (was reserved by %s) for new user %s on %s - GAP LIMIT PREVENTION",
				address, addr.Email, email, p.site)

			// Remove from old user
			if addr.Email != "" {
				delete(p.emailToAddress, addr.Email)
			}

			// Delete old expired payment records for this address before reassigning
			if p.persistence != nil && p.persistence.IsEnabled() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				paymentPersistence := NewPaymentPersistence()
				if paymentPersistence.IsEnabled() {
					err := paymentPersistence.DeleteExpiredPaymentsByAddress(ctx, address)
					if err != nil {
						log.Printf("⚠️ Failed to delete expired payment for address %s: %v", address, err)
					} else {
						log.Printf("🗑️ Deleted expired payment record for recycled address %s", address)
					}
				}
			}

			// Assign to new user
			addr.Email = email
			addr.ReservedAt = now
			addr.LastChecked = now
			addr.Status = AddressStatusReserved
			p.emailToAddress[email] = address

			// Save to database
			if p.persistence != nil && p.persistence.IsEnabled() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = p.persistence.SaveAddress(ctx, addr)
				_ = p.persistence.UpdateAddressReservation(ctx, address, email, now)
			}

			return address, nil
		}
	}

	// PRIORITY 3: Try global pool first (cross-site address sharing)
	if address, found := TakeFromGlobalPool(p.site); found {
		// Got address from global pool - need to set it up for this site
		pooledAddr := p.addresses[address]
		if pooledAddr == nil {
			// Address not in our tracking yet - create entry
			pooledAddr = &PooledAddress{
				Address:     address,
				Site:        p.site,
				Status:      AddressStatusAvailable,
				LastChecked: now,
			}
			p.addresses[address] = pooledAddr
		}

		// Assign to user
		pooledAddr.Email = email
		pooledAddr.ReservedAt = now
		pooledAddr.LastChecked = now
		pooledAddr.Status = AddressStatusReserved
		pooledAddr.Site = p.site // Update site assignment
		p.emailToAddress[email] = address

		// Save to database
		if p.persistence != nil && p.persistence.IsEnabled() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = p.persistence.SaveAddress(ctx, pooledAddr)
			_ = p.persistence.UpdateAddressReservation(ctx, address, email, now)
		}

		log.Printf("✅ Assigned address from GLOBAL pool to %s on %s", email, p.site)
		return address, nil
	}

	// PRIORITY 4: Get from site-specific available queue (legacy)
	if len(p.availableQueue) > 0 {
		address := p.availableQueue[0]
		p.availableQueue = p.availableQueue[1:]

		pooledAddr := p.addresses[address]

		// SMART RE-CHECK: Only verify if address has been sitting in pool for a while
		// Recent addresses (< 10 min) are trusted, old ones get re-checked for safety
		addressAge := now.Sub(pooledAddr.LastChecked)
		needsRecheck := addressAge > 10*time.Minute

		if needsRecheck {
			// Address has been in pool > 10 min, do quick safety check
			balance, txCount, err := payments.CheckAddressHistoryWithMempoolSpace(address)
			if err != nil {
				// If check fails, use address anyway (already verified during generation)
				log.Printf("⚠️ Warning: Could not re-verify old pooled address %s (age: %v): %v",
					address, addressAge.Round(time.Minute), err)
			} else if balance > 0 || txCount > 0 {
				// CRITICAL: Pooled address was compromised!
				log.Printf("🚨 CRITICAL: Old pooled address %s has HISTORY (balance: %d, txs: %d) - SKIPPING!",
					address, balance, txCount)
				log.Printf("🚨 Address age: %v - this should never happen!", addressAge.Round(time.Minute))

				// Try to get another address from pool
				if len(p.availableQueue) > 0 {
					return p.GetOrReuseAddress(email, amount) // Recursive call to try next address
				}
				// No more pool addresses, fall through to on-demand generation
				log.Printf("⚠️ Pool exhausted, generating on-demand for %s", email)
			} else {
				log.Printf("✅ Re-verified old pooled address %s (age: %v) - clean",
					address, addressAge.Round(time.Minute))
			}
		}

		// Address is clean (either recent or re-verified), assign it
		pooledAddr.Email = email
		pooledAddr.Status = AddressStatusReserved
		pooledAddr.ReservedAt = now
		pooledAddr.LastChecked = now

		p.emailToAddress[email] = address

		// Save to database
		if p.persistence != nil && p.persistence.IsEnabled() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = p.persistence.SaveAddress(ctx, pooledAddr)
			_ = p.persistence.RemoveFromQueue(ctx, p.site, address)
			_ = p.persistence.UpdateAddressReservation(ctx, address, email, now)
		}

		if needsRecheck {
			log.Printf("Assigned re-verified pooled address %s to %s on %s (pool size: %d remaining)",
				address, email, p.site, len(p.availableQueue))
		} else {
			log.Printf("Assigned pooled address %s to %s on %s (pool size: %d remaining) ⚡ INSTANT",
				address, email, p.site, len(p.availableQueue))
		}
		return address, nil
	}

	// PRIORITY 4: Generate on-demand when pool is empty
	// This is the main generation path now - no pre-generation needed
	log.Printf("Generating new address on-demand for %s on site %s", email, p.site)

	// Generate a single address at the current index
	config := SiteRegistry[p.site]
	if p.nextIndex > config.EndIndex {
		return "", fmt.Errorf("address index limit reached for site %s", p.site)
	}

	// Generate the address using the site-specific index
	// This will automatically skip addresses with transaction history
	address, actualIndex, err := generateAddressForSite(p.site, p.nextIndex)
	if err != nil {
		log.Printf("Failed to generate on-demand address for %s on %s: %v", email, p.site, err)
		return "", err
	}

	// Create new pooled address entry
	pooledAddr := &PooledAddress{
		Address:     address,
		Site:        p.site,
		Email:       email,
		Status:      AddressStatusReserved,
		ReservedAt:  now,
		LastChecked: now,
		Index:       actualIndex,
	}

	// Store in pool and update index to the next one after the actually used index
	p.addresses[address] = pooledAddr
	p.emailToAddress[email] = address
	p.nextIndex = actualIndex + 1

	// Register address-to-site mapping
	RegisterAddressForSite(address, p.site)

	// Save to database
	if p.persistence != nil && p.persistence.IsEnabled() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.persistence.SaveAddress(ctx, pooledAddr)
		_ = p.persistence.SavePoolState(ctx, p.site, p.nextIndex)
		_ = p.persistence.UpdateAddressReservation(ctx, address, email, now)
	}

	log.Printf("Generated on-demand address %s for %s on %s at index %d",
		address, email, p.site, pooledAddr.Index)

	return address, nil
}

// MarkAddressUsed - Called when payment is received
func (p *SiteAddressPool) MarkAddressUsed(address string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if addr, exists := p.addresses[address]; exists {
		addr.Status = AddressStatusUsed
		addr.PaymentCount++
		log.Printf("Address %s marked as USED for %s on %s (payment #%d)",
			address, addr.Email, p.site, addr.PaymentCount)

		// Remove from email mapping so user gets new address next time
		if addr.Email != "" {
			delete(p.emailToAddress, addr.Email)
		}

		// Save to database
		if p.persistence != nil && p.persistence.IsEnabled() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = p.persistence.MarkAddressUsed(ctx, address)
		}
	}
}

// RecycleExpiredAddresses - Run periodically (every hour)
func RecycleExpiredAddresses() {
	for siteName, pool := range sitePools {
		pool.mutex.Lock()

		now := time.Now()
		recycled := 0
		skipped := 0

		for address, addr := range pool.addresses {
			// Recycle addresses that are:
			// 1. Reserved (not paid) AND
			// 2. Older than 72 hours
			if addr.Status == AddressStatusReserved &&
				now.Sub(addr.ReservedAt) > 72*time.Hour {

				log.Printf("Attempting to recycle expired address %s from %s on %s (age: %v)",
					address, addr.Email, siteName, now.Sub(addr.ReservedAt))

				// CRITICAL: Check if address received payment during the 72 hours
				// Someone might have paid after the monitoring stopped
				balance, txCount, err := payments.CheckAddressHistoryWithMempoolSpace(address)
				if err != nil {
					log.Printf("⚠️ WARNING: Could not verify address %s during recycling: %v", address, err)
					log.Printf("⚠️ Skipping recycling to be safe (will retry next hour)")
					skipped++
					continue
				}

				if balance > 0 || txCount > 0 {
					// Address received payment! Don't recycle, mark as USED
					log.Printf("🚨 Address %s received late payment (balance: %d, txs: %d) - marking as USED instead of recycling",
						address, balance, txCount)

					addr.Status = AddressStatusUsed
					addr.PaymentCount++

					// Remove from email mapping
					if addr.Email != "" {
						delete(pool.emailToAddress, addr.Email)
					}

					// Save to database
					if pool.persistence != nil && pool.persistence.IsEnabled() {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						_ = pool.persistence.MarkAddressUsed(ctx, address)
					}

					skipped++
					continue
				}

				// Address is clean, safe to recycle
				log.Printf("✅ Address %s verified clean (0 balance, 0 txs) - recycling to pool", address)

				// Remove from current user
				if addr.Email != "" {
					delete(pool.emailToAddress, addr.Email)
				}

				// Mark as available and add back to queue
				addr.Status = AddressStatusAvailable
				addr.Email = ""
				addr.LastChecked = now // Update last checked time for age-based re-verification
				pool.availableQueue = append(pool.availableQueue, address)

				// Return to GLOBAL pool for cross-site reuse
				ReturnToGlobalPool(address)

				// Save to database
				if pool.persistence != nil && pool.persistence.IsEnabled() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_ = pool.persistence.UpdateAddressStatus(ctx, address, AddressStatusAvailable)
					_ = pool.persistence.AddToQueue(ctx, siteName, address)
				}

				recycled++
			}
		}

		pool.mutex.Unlock()

		if recycled > 0 || skipped > 0 {
			log.Printf("Recycling complete for site %s: %d recycled (clean), %d skipped (late payment/error)",
				siteName, recycled, skipped)
		}
	}
}

// GetGapLimitStatus - Monitor gap limit risk
func (p *SiteAddressPool) GetGapLimitStatus() (consecutiveUnpaid int, isAtRisk bool) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	config := SiteRegistry[p.site]
	unpaidStreak := 0
	maxStreak := 0

	// Check consecutive unpaid addresses (excluding skipped addresses with history)
	for i := config.StartIndex; i < p.nextIndex && i <= config.EndIndex; i++ {
		foundPaid := false
		foundSkipped := false

		for _, addr := range p.addresses {
			if addr.Index == i {
				if addr.Status == AddressStatusUsed {
					// Paid address - reset streak
					foundPaid = true
					unpaidStreak = 0
					break
				} else if addr.Status == AddressStatusSkipped {
					// Skipped address (has history) - treat as "paid" for gap limit purposes
					// This prevents skipped addresses from counting toward gap limit
					foundSkipped = true
					unpaidStreak = 0
					break
				}
			}
		}

		// Only count as unpaid if it's not paid AND not skipped
		if !foundPaid && !foundSkipped {
			unpaidStreak++
			if unpaidStreak > maxStreak {
				maxStreak = unpaidStreak
			}
		}
	}

	return maxStreak, maxStreak >= 15 // Alert at 15, critical at 20
}
