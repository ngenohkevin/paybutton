package payment_processing

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/ngenohkevin/paybutton/internals/payments"
)

// AddressPool manages a pool of pre-generated Bitcoin addresses
type AddressPool struct {
	mu              sync.RWMutex
	availableAddrs  []PoolAddress
	reservedAddrs   map[string]*PoolAddress
	usedAddrs       map[string]*PoolAddress
	minPoolSize     int
	maxPoolSize     int
	refillThreshold int
	lastRefill      time.Time
	refillCooldown  time.Duration
	persistFile     string
	stats           PoolStats
}

// PoolAddress represents an address in the pool
type PoolAddress struct {
	Address     string     `json:"address"`
	CreatedAt   time.Time  `json:"created_at"`
	ReservedAt  *time.Time `json:"reserved_at,omitempty"`
	ReservedFor string     `json:"reserved_for,omitempty"`
	UsedAt      *time.Time `json:"used_at,omitempty"`
	UsedBy      string     `json:"used_by,omitempty"`
	Amount      float64    `json:"amount,omitempty"`
}

// PoolStats tracks pool performance metrics
type PoolStats struct {
	TotalGenerated    int       `json:"total_generated"`
	TotalUsed         int       `json:"total_used"`
	TotalRecycled     int       `json:"total_recycled"`
	GapLimitErrors    int       `json:"gap_limit_errors"`
	LastGapLimitError time.Time `json:"last_gap_limit_error,omitempty"`
	CurrentPoolSize   int       `json:"current_pool_size"`
}

var (
	addressPool *AddressPool
	poolOnce    sync.Once
)

// InitializeAddressPool creates and initializes the global address pool
func InitializeAddressPool() *AddressPool {
	poolOnce.Do(func() {
		addressPool = &AddressPool{
			availableAddrs:  make([]PoolAddress, 0),
			reservedAddrs:   make(map[string]*PoolAddress),
			usedAddrs:       make(map[string]*PoolAddress),
			minPoolSize:     5,  // Minimum addresses to keep in pool
			maxPoolSize:     20, // Maximum addresses in pool
			refillThreshold: 3,  // Refill when pool drops to this size
			refillCooldown:  5 * time.Minute,
			persistFile:     "address_pool.json",
		}

		// Load persisted pool if exists
		addressPool.loadFromDisk()

		// Start background pool maintenance
		go addressPool.maintainPool()
	})
	return addressPool
}

// GetAddressPool returns the singleton address pool instance
func GetAddressPool() *AddressPool {
	if addressPool == nil {
		return InitializeAddressPool()
	}
	return addressPool
}

// ReserveAddress reserves an address from the pool for a user
func (p *AddressPool) ReserveAddress(email string, amount float64) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if user already has ANY reserved address within 72 hours
	// This prevents users from generating multiple addresses
	var existingReservedAddr string
	var mostRecentReservation *time.Time
	
	for addr, poolAddr := range p.reservedAddrs {
		if poolAddr.ReservedFor == email && poolAddr.ReservedAt != nil {
			// Track the most recent reservation
			if mostRecentReservation == nil || poolAddr.ReservedAt.After(*mostRecentReservation) {
				mostRecentReservation = poolAddr.ReservedAt
				existingReservedAddr = addr
			}
		}
	}
	
	// If user has a reserved address within 72 hours, return that same address
	if mostRecentReservation != nil && time.Since(*mostRecentReservation) < 72*time.Hour {
		// Update the reservation time and amount
		if poolAddr, exists := p.reservedAddrs[existingReservedAddr]; exists {
			now := time.Now()
			poolAddr.ReservedAt = &now
			poolAddr.Amount = amount
			log.Printf("User %s already has reserved address %s (reserved %v ago), returning same address", 
				email, existingReservedAddr, time.Since(*mostRecentReservation).Round(time.Minute))
			return existingReservedAddr, nil
		}
	}

	// Note: We removed the check for used addresses here
	// Users who have successfully paid SHOULD be able to get a new address for their next payment
	// The 72-hour reservation on their previous address is already handled above

	// Get address from pool
	if len(p.availableAddrs) == 0 {
		// Try to generate one address urgently
		addr, err := p.generateSingleAddress()
		if err != nil {
			p.stats.GapLimitErrors++
			p.stats.LastGapLimitError = time.Now()
			return "", fmt.Errorf("no addresses available in pool and failed to generate: %v", err)
		}

		now := time.Now()
		poolAddr := &PoolAddress{
			Address:     addr,
			CreatedAt:   now,
			ReservedAt:  &now,
			ReservedFor: email,
			Amount:      amount,
		}
		p.reservedAddrs[addr] = poolAddr
		return addr, nil
	}

	// Take address from pool, but ensure it's not already used
	var poolAddr PoolAddress
	var foundCleanAddress bool
	
	for i, addr := range p.availableAddrs {
		// Check if this address is NOT in usedAddrs
		if _, isUsed := p.usedAddrs[addr.Address]; !isUsed {
			poolAddr = addr
			// Remove from available pool
			p.availableAddrs = append(p.availableAddrs[:i], p.availableAddrs[i+1:]...)
			foundCleanAddress = true
			break
		} else {
			log.Printf("WARNING: Found used address %s in available pool, skipping", addr.Address)
		}
	}
	
	if !foundCleanAddress {
		log.Printf("No clean addresses available in pool for user %s", email)
		return "", fmt.Errorf("no clean addresses available in pool")
	}

	now := time.Now()
	poolAddr.ReservedAt = &now
	poolAddr.ReservedFor = email
	poolAddr.Amount = amount

	p.reservedAddrs[poolAddr.Address] = &poolAddr
	p.stats.CurrentPoolSize = len(p.availableAddrs)

	log.Printf("Reserved address %s from pool for user %s (pool size: %d)",
		poolAddr.Address, email, len(p.availableAddrs))

	// Trigger refill if needed
	if len(p.availableAddrs) <= p.refillThreshold {
		go p.refillPool()
	}

	return poolAddr.Address, nil
}

// MarkAddressUsed marks an address as used (payment received)
func (p *AddressPool) MarkAddressUsed(address string, email string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if poolAddr, exists := p.reservedAddrs[address]; exists {
		now := time.Now()
		poolAddr.UsedAt = &now
		poolAddr.UsedBy = email

		p.usedAddrs[address] = poolAddr
		delete(p.reservedAddrs, address)

		p.stats.TotalUsed++
		log.Printf("Marked address %s as used by %s", address, email)
	}
}

// recycleExpiredReservationsInternal returns expired reserved addresses back to pool (internal method)
func (p *AddressPool) recycleExpiredReservationsInternal() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	recycled := 0
	usedAddressesFound := 0

	for addr, poolAddr := range p.reservedAddrs {
		// Only recycle if reserved for more than 72 hours without payment
		// This gives users plenty of time to complete their payment
		if poolAddr.ReservedAt != nil && now.Sub(*poolAddr.ReservedAt) > 72*time.Hour {
			// Before recycling, check if address has received funds
			hasBalance := p.checkAddressBalance(addr)
			if hasBalance {
				// Move to used addresses instead of recycling
				now := time.Now()
				poolAddr.UsedAt = &now
				poolAddr.UsedBy = poolAddr.ReservedFor
				p.usedAddrs[addr] = poolAddr
				p.stats.TotalUsed++
				usedAddressesFound++
				log.Printf("Moved funded address %s to used addresses (reserved by %s)", addr, poolAddr.ReservedFor)
			} else {
				// Only recycle if address is less than 7 days old and has no funds
				if now.Sub(poolAddr.CreatedAt) < 7*24*time.Hour {
					// Double-check this address is not in usedAddrs before recycling
					if _, isUsed := p.usedAddrs[addr]; !isUsed {
						// Reset reservation info before adding back to pool
						poolAddr.ReservedAt = nil
						poolAddr.ReservedFor = ""
						poolAddr.Amount = 0
						p.availableAddrs = append(p.availableAddrs, *poolAddr)
						recycled++
						log.Printf("Recycled unused address %s after 72 hours (originally for %s)", addr, poolAddr.ReservedFor)
					} else {
						log.Printf("WARNING: Prevented recycling of used address %s", addr)
					}
				} else {
					// Address is too old (>7 days), don't recycle it
					log.Printf("Discarding old unused address %s (created %v ago)", addr, now.Sub(poolAddr.CreatedAt))
				}
			}
			delete(p.reservedAddrs, addr)
		}
	}

	if recycled > 0 || usedAddressesFound > 0 {
		p.stats.TotalRecycled += recycled
		p.stats.CurrentPoolSize = len(p.availableAddrs)
		log.Printf("Pool maintenance: recycled %d clean addresses, moved %d funded addresses to used", recycled, usedAddressesFound)
	}
}

// refillPool generates new addresses to maintain pool size
func (p *AddressPool) refillPool() {
	p.mu.Lock()

	// Check cooldown
	if time.Since(p.lastRefill) < p.refillCooldown {
		p.mu.Unlock()
		return
	}

	needed := p.minPoolSize - len(p.availableAddrs)
	if needed <= 0 {
		p.mu.Unlock()
		return
	}

	// Don't exceed max pool size
	if len(p.availableAddrs)+len(p.reservedAddrs) >= p.maxPoolSize {
		p.mu.Unlock()
		return
	}

	p.lastRefill = time.Now()
	p.mu.Unlock()

	log.Printf("Refilling address pool, generating %d addresses...", needed)

	generated := 0
	failures := 0

	for i := 0; i < needed && failures < 3; i++ {
		addr, err := p.generateSingleAddress()
		if err != nil {
			failures++
			log.Printf("Failed to generate address during refill: %v", err)

			// If gap limit error, stop trying
			if isGapLimitError(err) {
				p.mu.Lock()
				p.stats.GapLimitErrors++
				p.stats.LastGapLimitError = time.Now()
				p.mu.Unlock()
				break
			}
			continue
		}

		p.mu.Lock()
		// Before adding to available pool, verify it's not in usedAddrs
		if _, isUsed := p.usedAddrs[addr]; !isUsed {
			p.availableAddrs = append(p.availableAddrs, PoolAddress{
				Address:   addr,
				CreatedAt: time.Now(),
			})
			p.stats.TotalGenerated++
			p.stats.CurrentPoolSize = len(p.availableAddrs)
		} else {
			log.Printf("WARNING: Attempted to add used address %s to available pool during refill", addr)
		}
		p.mu.Unlock()

		generated++

		// Small delay between generations to avoid rate limiting
		time.Sleep(500 * time.Millisecond)
	}

	if generated > 0 {
		log.Printf("Successfully generated %d addresses for pool", generated)
		p.persistToDisk()
	}
}

// generateSingleAddress generates a single Bitcoin address
func (p *AddressPool) generateSingleAddress() (string, error) {
	// Use a dummy email and amount for pool addresses
	addr, err := payments.GenerateBitcoinAddress("pool@system", 0.001)
	if err != nil {
		return "", err
	}
	return addr, nil
}

// maintainPool runs periodic maintenance tasks
func (p *AddressPool) maintainPool() {
	// Runs every 5 minutes to check for expired reservations
	// But addresses are only recycled after 72 hours of no payment
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		p.recycleExpiredReservationsInternal()

		// Refill pool during low-traffic hours (2-6 AM UTC)
		hour := time.Now().UTC().Hour()
		if hour >= 2 && hour <= 6 {
			p.refillPool()
		}

		// Persist current state
		p.persistToDisk()
	}
}

// GetStats returns current pool statistics
func (p *AddressPool) GetStats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := p.stats
	stats.CurrentPoolSize = len(p.availableAddrs)
	return stats
}

// persistToDisk saves pool state to disk
func (p *AddressPool) persistToDisk() {
	p.mu.RLock()
	defer p.mu.RUnlock()

	data := map[string]interface{}{
		"available": p.availableAddrs,
		"reserved":  p.reservedAddrs,
		"used":      p.usedAddrs,
		"stats":     p.stats,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal pool data: %v", err)
		return
	}

	err = os.WriteFile(p.persistFile, jsonData, 0644)
	if err != nil {
		log.Printf("Failed to persist pool data: %v", err)
	}
}

// loadFromDisk loads pool state from disk
func (p *AddressPool) loadFromDisk() {
	data, err := os.ReadFile(p.persistFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Failed to load pool data: %v", err)
		}
		return
	}

	var poolData map[string]json.RawMessage
	if err := json.Unmarshal(data, &poolData); err != nil {
		log.Printf("Failed to unmarshal pool data: %v", err)
		return
	}

	if available, exists := poolData["available"]; exists {
		json.Unmarshal(available, &p.availableAddrs)
	}

	if reserved, exists := poolData["reserved"]; exists {
		json.Unmarshal(reserved, &p.reservedAddrs)
	}

	if used, exists := poolData["used"]; exists {
		json.Unmarshal(used, &p.usedAddrs)
	}

	if stats, exists := poolData["stats"]; exists {
		json.Unmarshal(stats, &p.stats)
	}

	log.Printf("Loaded address pool from disk: %d available, %d reserved, %d used",
		len(p.availableAddrs), len(p.reservedAddrs), len(p.usedAddrs))
}

// ForceRefill triggers a manual pool refill, bypassing cooldown restrictions
func (p *AddressPool) ForceRefill() error {
	log.Printf("Manual pool refill requested")

	p.mu.Lock()
	needed := p.minPoolSize - len(p.availableAddrs)

	// Allow refill even if we're at minimum, for manual requests
	if needed <= 0 {
		needed = 5 // Generate at least 5 new addresses for manual refill
	}

	// Don't exceed max pool size
	maxGenerate := p.maxPoolSize - (len(p.availableAddrs) + len(p.reservedAddrs))
	if maxGenerate <= 0 {
		p.mu.Unlock()
		return fmt.Errorf("pool is already at maximum capacity")
	}

	if needed > maxGenerate {
		needed = maxGenerate
	}

	// Reset cooldown for manual refill
	p.lastRefill = time.Time{}
	p.mu.Unlock()

	// Trigger refill in a goroutine to avoid blocking
	go func() {
		log.Printf("Manual refill starting, generating %d addresses...", needed)

		generated := 0
		failures := 0

		for i := 0; i < needed && failures < 5; i++ {
			addr, err := p.generateSingleAddress()
			if err != nil {
				failures++
				log.Printf("Failed to generate address during manual refill: %v", err)

				// If gap limit error, stop trying
				if isGapLimitError(err) {
					p.mu.Lock()
					p.stats.GapLimitErrors++
					p.stats.LastGapLimitError = time.Now()
					p.mu.Unlock()
					break
				}
				continue
			}

			p.mu.Lock()
			p.availableAddrs = append(p.availableAddrs, PoolAddress{
				Address:   addr,
				CreatedAt: time.Now(),
			})
			p.stats.TotalGenerated++
			p.stats.CurrentPoolSize = len(p.availableAddrs)
			p.mu.Unlock()

			generated++
		}

		if generated > 0 {
			log.Printf("Manual refill completed: generated %d addresses", generated)
			p.persistToDisk()
		} else if failures > 0 {
			log.Printf("Manual refill failed: could not generate any addresses after %d failures", failures)
		}
	}()

	return nil
}

// GetDetailedInfo returns comprehensive pool information for the management interface
func (p *AddressPool) GetDetailedInfo() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Convert internal structs to public format
	available := make([]map[string]interface{}, len(p.availableAddrs))
	for i, addr := range p.availableAddrs {
		available[i] = map[string]interface{}{
			"address":    addr.Address,
			"created_at": addr.CreatedAt,
		}
	}

	reserved := make([]map[string]interface{}, 0)
	for _, addr := range p.reservedAddrs {
		item := map[string]interface{}{
			"address":      addr.Address,
			"created_at":   addr.CreatedAt,
			"reserved_at":  addr.ReservedAt,
			"reserved_for": addr.ReservedFor,
		}
		reserved = append(reserved, item)
	}

	used := make([]map[string]interface{}, 0)
	for _, addr := range p.usedAddrs {
		item := map[string]interface{}{
			"address":    addr.Address,
			"created_at": addr.CreatedAt,
			"used_at":    addr.UsedAt,
			"used_by":    addr.UsedBy,
			"amount":     addr.Amount,
		}
		used = append(used, item)
	}

	return map[string]interface{}{
		"stats": p.stats,
		"config": map[string]interface{}{
			"min_pool_size":    p.minPoolSize,
			"max_pool_size":    p.maxPoolSize,
			"refill_threshold": p.refillThreshold,
			"refill_cooldown":  p.refillCooldown.String(),
		},
		"available": available,
		"reserved":  reserved,
		"used":      used,
	}
}

// ReleaseReservation releases a specific reserved address back to available pool
func (p *AddressPool) ReleaseReservation(address string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if reservedAddr, exists := p.reservedAddrs[address]; exists {
		// Move back to available
		p.availableAddrs = append(p.availableAddrs, *reservedAddr)
		delete(p.reservedAddrs, address)
		p.stats.CurrentPoolSize = len(p.availableAddrs)

		log.Printf("Released reservation for address %s", address)
		p.persistToDisk()
		return true
	}

	return false
}

// ClearUnusedAddresses removes unused addresses from the pool (dangerous operation)
func (p *AddressPool) ClearUnusedAddresses() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	clearedCount := len(p.availableAddrs)
	p.availableAddrs = []PoolAddress{}
	p.stats.CurrentPoolSize = 0

	log.Printf("Cleared %d unused addresses from pool", clearedCount)
	p.persistToDisk()
	return clearedCount
}

// UpdateConfiguration updates pool configuration parameters
func (p *AddressPool) UpdateConfiguration(minSize, maxSize, refillThreshold int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.minPoolSize = minSize
	p.maxPoolSize = maxSize
	p.refillThreshold = refillThreshold

	log.Printf("Updated pool configuration: min=%d, max=%d, threshold=%d", minSize, maxSize, refillThreshold)
	p.persistToDisk()
	return nil
}

// ExportData exports all pool data for backup/analysis
func (p *AddressPool) ExportData() map[string]interface{} {
	return p.GetDetailedInfo()
}

// ExportUsedAddressesCSV exports used addresses in CSV format
func (p *AddressPool) ExportUsedAddressesCSV(filter string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	csv := "Address,Used At,Used By,Amount BTC,Created At\n"

	for _, addr := range p.usedAddrs {
		if addr.UsedAt == nil {
			continue
		}

		// Apply time filter
		if filter != "all" {
			now := time.Now()
			switch filter {
			case "today":
				if addr.UsedAt.Format("2006-01-02") != now.Format("2006-01-02") {
					continue
				}
			case "week":
				weekAgo := now.AddDate(0, 0, -7)
				if addr.UsedAt.Before(weekAgo) {
					continue
				}
			case "month":
				monthAgo := now.AddDate(0, -1, 0)
				if addr.UsedAt.Before(monthAgo) {
					continue
				}
			}
		}

		usedBy := addr.UsedBy
		if usedBy == "" {
			usedBy = "Unknown"
		}

		csv += fmt.Sprintf("%s,%s,%s,%.8f,%s\n",
			addr.Address,
			addr.UsedAt.Format("2006-01-02 15:04:05"),
			usedBy,
			addr.Amount,
			addr.CreatedAt.Format("2006-01-02 15:04:05"),
		)
	}

	return csv
}

// checkAddressBalance verifies if an address has received funds
func (p *AddressPool) checkAddressBalance(address string) bool {
	// Use the existing balance checking function from balance_ops.go
	balance, err := GetBitcoinAddressBalanceWithFallback(address, blockCypherToken)
	if err != nil {
		// If we can't check balance, be conservative and assume it might have funds
		log.Printf("Warning: Could not check balance for address %s during recycling: %v", address, err)
		return true
	}
	
	// Consider any balance > 0 satoshis as "has funds"
	return balance > 0
}

// RecycleExpiredReservations returns the count of recycled addresses for admin feedback
func (p *AddressPool) RecycleExpiredReservations() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	recycled := 0
	reservationTimeout := 24 * time.Hour // 24 hours timeout

	for address, addr := range p.reservedAddrs {
		if addr.ReservedAt != nil && time.Since(*addr.ReservedAt) > reservationTimeout {
			// Move back to available
			addr.ReservedAt = nil
			addr.ReservedFor = ""
			p.availableAddrs = append(p.availableAddrs, *addr)
			delete(p.reservedAddrs, address)
			recycled++
		}
	}

	if recycled > 0 {
		p.stats.TotalRecycled += recycled
		p.stats.CurrentPoolSize = len(p.availableAddrs)
		log.Printf("Recycled %d expired reserved addresses back to pool", recycled)
		p.persistToDisk()
	}

	return recycled
}
