package payment_processing

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type AddressStatus string

const (
	AddressStatusAvailable AddressStatus = "available"  // Can be assigned
	AddressStatusReserved  AddressStatus = "reserved"   // Assigned, waiting for payment
	AddressStatusUsed      AddressStatus = "used"       // Payment received
	AddressStatusExpired   AddressStatus = "expired"    // 72h passed, ready to recycle
)

type PooledAddress struct {
	Address      string
	Site         string
	Email        string        // Current or last user
	Status       AddressStatus
	ReservedAt   time.Time     // When it was assigned
	LastChecked  time.Time     // Last balance check
	PaymentCount int           // How many times it received payment
	Index        int           // Address index in HD wallet
}

type SiteAddressPool struct {
	site            string
	addresses       map[string]*PooledAddress  // address -> details
	emailToAddress  map[string]string          // email -> current address
	availableQueue  []string                   // FIFO queue of available addresses
	nextIndex       int                        // Next index to generate
	mutex           sync.RWMutex
}

var (
	sitePools = make(map[string]*SiteAddressPool)
	poolMutex sync.RWMutex
)

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

// GetOrReuseAddress - Primary function for address assignment with aggressive reuse
func (p *SiteAddressPool) GetOrReuseAddress(email string, amount float64) (string, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// PRIORITY 1: Check if user already has an unpaid address for this site
	if existingAddr, exists := p.emailToAddress[email]; exists {
		if addr := p.addresses[existingAddr]; addr != nil {
			if addr.Status == AddressStatusReserved {
				// Still unpaid, MUST REUSE to prevent gap limit!
				addr.LastChecked = time.Now()
				log.Printf("REUSING unpaid address %s for %s on %s (reserved %v ago) - GAP LIMIT PREVENTION",
					existingAddr, email, p.site, time.Since(addr.ReservedAt))
				return existingAddr, nil
			}
		}
	}

	// PRIORITY 2: Find any expired address (72h old, unpaid) to recycle
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

			// Assign to new user
			addr.Email = email
			addr.ReservedAt = now
			addr.LastChecked = now
			addr.Status = AddressStatusReserved
			p.emailToAddress[email] = address

			return address, nil
		}
	}

	// PRIORITY 3: Get from available queue (pre-generated addresses)
	if len(p.availableQueue) > 0 {
		address := p.availableQueue[0]
		p.availableQueue = p.availableQueue[1:]

		pooledAddr := p.addresses[address]
		pooledAddr.Email = email
		pooledAddr.Status = AddressStatusReserved
		pooledAddr.ReservedAt = now
		pooledAddr.LastChecked = now

		p.emailToAddress[email] = address

		log.Printf("Assigned pooled address %s to %s on %s (pool size: %d remaining)",
			address, email, p.site, len(p.availableQueue))
		return address, nil
	}

	// PRIORITY 4: Only generate new if absolutely necessary
	// This should be VERY RARE - only when pool is exhausted
	return "", fmt.Errorf("address pool exhausted for site %s - need to pre-generate more", p.site)
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
	}
}

// RecycleExpiredAddresses - Run periodically (every hour)
func RecycleExpiredAddresses() {
	for siteName, pool := range sitePools {
		pool.mutex.Lock()

		now := time.Now()
		recycled := 0

		for address, addr := range pool.addresses {
			// Recycle addresses that are:
			// 1. Reserved (not paid) AND
			// 2. Older than 72 hours
			if addr.Status == AddressStatusReserved &&
			   now.Sub(addr.ReservedAt) > 72*time.Hour {

				log.Printf("Recycling expired address %s from %s on %s (age: %v)",
					address, addr.Email, siteName, now.Sub(addr.ReservedAt))

				// Remove from current user
				if addr.Email != "" {
					delete(pool.emailToAddress, addr.Email)
				}

				// Mark as available and add back to queue
				addr.Status = AddressStatusAvailable
				addr.Email = ""
				pool.availableQueue = append(pool.availableQueue, address)
				recycled++
			}
		}

		pool.mutex.Unlock()

		if recycled > 0 {
			log.Printf("Recycled %d addresses for site %s - preventing gap limit", recycled, siteName)
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

	// Check consecutive unpaid addresses
	for i := config.StartIndex; i < p.nextIndex && i <= config.EndIndex; i++ {
		found := false
		for _, addr := range p.addresses {
			if addr.Index == i && addr.Status == AddressStatusUsed {
				found = true
				unpaidStreak = 0
				break
			}
		}
		if !found {
			unpaidStreak++
			if unpaidStreak > maxStreak {
				maxStreak = unpaidStreak
			}
		}
	}

	return maxStreak, maxStreak >= 15 // Alert at 15, critical at 20
}