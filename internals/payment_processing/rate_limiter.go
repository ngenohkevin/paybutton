package payment_processing

import (
	"fmt"
	"sync"
	"time"
)

// RateLimiter implements a token bucket rate limiter for address generation
type RateLimiter struct {
	mu              sync.Mutex
	ipLimits        map[string]*TokenBucket
	emailLimits     map[string]*TokenBucket
	globalBucket    *TokenBucket
	cleanupInterval time.Duration
}

// TokenBucket represents a token bucket for rate limiting
type TokenBucket struct {
	tokens         int
	maxTokens      int
	refillRate     int           // tokens per interval
	refillInterval time.Duration
	lastRefill     time.Time
	lastAccess     time.Time
}

var (
	rateLimiter     *RateLimiter
	rateLimiterOnce sync.Once
)

// InitializeRateLimiter creates and initializes the global rate limiter
func InitializeRateLimiter() *RateLimiter {
	rateLimiterOnce.Do(func() {
		rateLimiter = &RateLimiter{
			ipLimits:        make(map[string]*TokenBucket),
			emailLimits:     make(map[string]*TokenBucket),
			cleanupInterval: 30 * time.Minute,
			globalBucket: &TokenBucket{
				tokens:         100,
				maxTokens:      100,
				refillRate:     50,
				refillInterval: time.Hour,
				lastRefill:     time.Now(),
			},
		}
		
		// Start cleanup goroutine
		go rateLimiter.cleanup()
	})
	return rateLimiter
}

// GetRateLimiter returns the singleton rate limiter instance
func GetRateLimiter() *RateLimiter {
	if rateLimiter == nil {
		return InitializeRateLimiter()
	}
	return rateLimiter
}

// AllowAddressGeneration checks if address generation is allowed for the given IP and email
func (r *RateLimiter) AllowAddressGeneration(ip, email string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Check global rate limit first
	if !r.consumeToken(r.globalBucket) {
		return false, fmt.Errorf("global rate limit exceeded, please try again later")
	}
	
	// Check IP rate limit
	ipBucket := r.getOrCreateBucket(r.ipLimits, ip, 10, 5, 30*time.Minute)
	if !r.consumeToken(ipBucket) {
		// Refund global token
		r.globalBucket.tokens++
		return false, fmt.Errorf("too many requests from your IP, please wait before trying again")
	}
	
	// Check email rate limit
	emailBucket := r.getOrCreateBucket(r.emailLimits, email, 5, 3, time.Hour)
	if !r.consumeToken(emailBucket) {
		// Refund tokens
		r.globalBucket.tokens++
		ipBucket.tokens++
		return false, fmt.Errorf("too many address requests for this email, please wait before requesting more")
	}
	
	return true, nil
}

// getOrCreateBucket gets or creates a token bucket for the given key
func (r *RateLimiter) getOrCreateBucket(buckets map[string]*TokenBucket, key string, maxTokens, refillRate int, refillInterval time.Duration) *TokenBucket {
	bucket, exists := buckets[key]
	if !exists {
		bucket = &TokenBucket{
			tokens:         maxTokens,
			maxTokens:      maxTokens,
			refillRate:     refillRate,
			refillInterval: refillInterval,
			lastRefill:     time.Now(),
			lastAccess:     time.Now(),
		}
		buckets[key] = bucket
	}
	
	// Refill tokens if needed
	r.refillBucket(bucket)
	bucket.lastAccess = time.Now()
	
	return bucket
}

// refillBucket refills tokens in the bucket based on elapsed time
func (r *RateLimiter) refillBucket(bucket *TokenBucket) {
	now := time.Now()
	elapsed := now.Sub(bucket.lastRefill)
	
	if elapsed >= bucket.refillInterval {
		// Calculate how many intervals have passed
		intervals := int(elapsed / bucket.refillInterval)
		tokensToAdd := intervals * bucket.refillRate
		
		bucket.tokens += tokensToAdd
		if bucket.tokens > bucket.maxTokens {
			bucket.tokens = bucket.maxTokens
		}
		
		// Update last refill time
		bucket.lastRefill = bucket.lastRefill.Add(time.Duration(intervals) * bucket.refillInterval)
	}
}

// consumeToken attempts to consume a token from the bucket
func (r *RateLimiter) consumeToken(bucket *TokenBucket) bool {
	if bucket.tokens > 0 {
		bucket.tokens--
		return true
	}
	return false
}

// cleanup removes inactive buckets to prevent memory leaks
func (r *RateLimiter) cleanup() {
	ticker := time.NewTicker(r.cleanupInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		r.mu.Lock()
		
		now := time.Now()
		inactiveThreshold := 2 * time.Hour
		
		// Clean up IP buckets
		for ip, bucket := range r.ipLimits {
			if now.Sub(bucket.lastAccess) > inactiveThreshold {
				delete(r.ipLimits, ip)
			}
		}
		
		// Clean up email buckets
		for email, bucket := range r.emailLimits {
			if now.Sub(bucket.lastAccess) > inactiveThreshold {
				delete(r.emailLimits, email)
			}
		}
		
		r.mu.Unlock()
	}
}

// GetStats returns current rate limiter statistics
func (r *RateLimiter) GetStats() map[string]interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	return map[string]interface{}{
		"global_tokens":  r.globalBucket.tokens,
		"ip_limits":      len(r.ipLimits),
		"email_limits":   len(r.emailLimits),
		"global_max":     r.globalBucket.maxTokens,
	}
}

// ResetLimits resets rate limits for a specific email (admin function)
func (r *RateLimiter) ResetLimits(email string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if bucket, exists := r.emailLimits[email]; exists {
		bucket.tokens = bucket.maxTokens
		bucket.lastRefill = time.Now()
	}
}