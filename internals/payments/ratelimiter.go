package payments

import (
	"context"
	"sync"
	"time"
)

// APIRateLimiter manages rate limiting for blockchain API providers
type APIRateLimiter struct {
	providers map[string]*ProviderLimiter
	mu        sync.RWMutex
}

// ProviderLimiter handles rate limiting for a single API provider
type ProviderLimiter struct {
	name        string
	maxRequests int
	window      time.Duration
	requests    chan struct{}
	lastRequest time.Time
	minInterval time.Duration
	mu          sync.Mutex
}

var (
	globalRateLimiter *APIRateLimiter
	once              sync.Once
)

// GetRateLimiter returns the global rate limiter instance
func GetRateLimiter() *APIRateLimiter {
	once.Do(func() {
		globalRateLimiter = &APIRateLimiter{
			providers: make(map[string]*ProviderLimiter),
		}

		// Initialize rate limiters for each provider
		// Fastest providers get most aggressive rate limits for speed
		globalRateLimiter.AddProvider("mempoolspace", 60, time.Minute, 1*time.Second) // 60 req/min, min 1s between calls (fastest)
		globalRateLimiter.AddProvider("blockstream", 50, time.Minute, 1*time.Second)  // 50 req/min, min 1s between calls (very fast)
		globalRateLimiter.AddProvider("trezor", 30, time.Minute, 2*time.Second)       // 30 req/min, min 2s between calls (fast)
		globalRateLimiter.AddProvider("blockchain", 10, time.Minute, 6*time.Second)   // 10 req/min, min 6s between calls
		globalRateLimiter.AddProvider("blockcypher", 20, time.Hour, 3*time.Minute)    // 20 req/hour, min 3m between calls
		globalRateLimiter.AddProvider("blockonomics", 5, time.Minute, 12*time.Second) // 5 req/min, min 12s between calls (slowest)
	})
	return globalRateLimiter
}

// AddProvider adds a new provider with rate limiting configuration
func (rl *APIRateLimiter) AddProvider(name string, maxRequests int, window time.Duration, minInterval time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.providers[name] = &ProviderLimiter{
		name:        name,
		maxRequests: maxRequests,
		window:      window,
		requests:    make(chan struct{}, maxRequests),
		minInterval: minInterval,
	}

	// Fill the initial quota
	for i := 0; i < maxRequests; i++ {
		rl.providers[name].requests <- struct{}{}
	}

	// Start the quota refill goroutine
	go rl.providers[name].refillQuota()
}

// WaitForPermission waits until it's safe to make a request to the specified provider
func (rl *APIRateLimiter) WaitForPermission(ctx context.Context, provider string) error {
	rl.mu.RLock()
	limiter, exists := rl.providers[provider]
	rl.mu.RUnlock()

	if !exists {
		// If provider doesn't exist, allow the request
		return nil
	}

	return limiter.waitForPermission(ctx)
}

// waitForPermission waits for permission to make a request
func (pl *ProviderLimiter) waitForPermission(ctx context.Context) error {
	// Check minimum interval since last request
	pl.mu.Lock()
	timeSinceLastRequest := time.Since(pl.lastRequest)
	if timeSinceLastRequest < pl.minInterval {
		waitTime := pl.minInterval - timeSinceLastRequest
		pl.mu.Unlock()

		// Wait for the minimum interval
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// Continue to quota check
		}
	} else {
		pl.mu.Unlock()
	}

	// Wait for quota availability
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-pl.requests:
		// Got permission, update last request time
		pl.mu.Lock()
		pl.lastRequest = time.Now()
		pl.mu.Unlock()
		return nil
	}
}

// refillQuota periodically refills the request quota
func (pl *ProviderLimiter) refillQuota() {
	ticker := time.NewTicker(pl.window / time.Duration(pl.maxRequests))
	defer ticker.Stop()

	for range ticker.C {
		select {
		case pl.requests <- struct{}{}:
			// Successfully added a token
		default:
			// Channel is full, skip this refill
		}
	}
}

// GetProviderStatus returns the current status of a provider's rate limiter
func (rl *APIRateLimiter) GetProviderStatus(provider string) (available int, total int) {
	rl.mu.RLock()
	limiter, exists := rl.providers[provider]
	rl.mu.RUnlock()

	if !exists {
		return -1, -1
	}

	return len(limiter.requests), limiter.maxRequests
}
