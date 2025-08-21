package payments

import (
	"sync"
	"time"
)

// CacheEntry represents a cached balance entry
type CacheEntry struct {
	Balance   int64
	Timestamp time.Time
}

// BalanceCache manages cached balance responses
type BalanceCache struct {
	entries map[string]*CacheEntry
	ttl     time.Duration
	mu      sync.RWMutex
}

var (
	globalBalanceCache *BalanceCache
	cacheOnce          sync.Once
)

// GetBalanceCache returns the global balance cache instance
func GetBalanceCache() *BalanceCache {
	cacheOnce.Do(func() {
		globalBalanceCache = &BalanceCache{
			entries: make(map[string]*CacheEntry),
			ttl:     15 * time.Second, // Cache for 15 seconds - faster detection
		}

		// Start cleanup goroutine
		go globalBalanceCache.cleanup()
	})
	return globalBalanceCache
}

// Get retrieves a cached balance if it exists and is still valid
func (bc *BalanceCache) Get(address string) (int64, bool) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	entry, exists := bc.entries[address]
	if !exists {
		return 0, false
	}

	// Check if entry has expired
	if time.Since(entry.Timestamp) > bc.ttl {
		return 0, false
	}

	return entry.Balance, true
}

// Set stores a balance in the cache
func (bc *BalanceCache) Set(address string, balance int64) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.entries[address] = &CacheEntry{
		Balance:   balance,
		Timestamp: time.Now(),
	}
}

// Delete removes an entry from the cache
func (bc *BalanceCache) Delete(address string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	delete(bc.entries, address)
}

// Clear removes all entries from the cache
func (bc *BalanceCache) Clear() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.entries = make(map[string]*CacheEntry)
}

// GetStats returns cache statistics
func (bc *BalanceCache) GetStats() (total int, expired int) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	total = len(bc.entries)
	now := time.Now()

	for _, entry := range bc.entries {
		if now.Sub(entry.Timestamp) > bc.ttl {
			expired++
		}
	}

	return total, expired
}

// cleanup periodically removes expired entries
func (bc *BalanceCache) cleanup() {
	ticker := time.NewTicker(60 * time.Second) // Cleanup every minute
	defer ticker.Stop()

	for range ticker.C {
		bc.mu.Lock()
		now := time.Now()
		for address, entry := range bc.entries {
			if now.Sub(entry.Timestamp) > bc.ttl {
				delete(bc.entries, address)
			}
		}
		bc.mu.Unlock()
	}
}
