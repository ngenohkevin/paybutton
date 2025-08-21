package payments

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// CircuitState represents the current state of a circuit breaker
type CircuitState int

const (
	StateClosed CircuitState = iota
	StateHalfOpen
	StateOpen
)

// CircuitBreaker implements the circuit breaker pattern for API providers
type CircuitBreaker struct {
	name            string
	maxFailures     int
	resetTimeout    time.Duration
	state           CircuitState
	failureCount    int
	lastFailureTime time.Time
	mu              sync.RWMutex
}

// CircuitBreakerManager manages circuit breakers for all API providers
type CircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
}

var (
	globalCircuitManager *CircuitBreakerManager
	circuitOnce          sync.Once
)

// GetCircuitBreakerManager returns the global circuit breaker manager
func GetCircuitBreakerManager() *CircuitBreakerManager {
	circuitOnce.Do(func() {
		globalCircuitManager = &CircuitBreakerManager{
			breakers: make(map[string]*CircuitBreaker),
		}

		// Initialize circuit breakers for each provider
		globalCircuitManager.AddProvider("blockchain", 5, 2*time.Minute)   // Allow 5 failures, reset after 2 minutes
		globalCircuitManager.AddProvider("blockonomics", 3, 3*time.Minute) // Allow 3 failures, reset after 3 minutes
		globalCircuitManager.AddProvider("blockcypher", 5, 5*time.Minute)  // Allow 5 failures, reset after 5 minutes
		globalCircuitManager.AddProvider("blockstream", 4, 1*time.Minute)  // Allow 4 failures, reset after 1 minute (reliable)
		globalCircuitManager.AddProvider("mempoolspace", 4, 1*time.Minute) // Allow 4 failures, reset after 1 minute (reliable)
		globalCircuitManager.AddProvider("trezor", 3, 2*time.Minute)       // Allow 3 failures, reset after 2 minutes
	})
	return globalCircuitManager
}

// AddProvider adds a new circuit breaker for a provider
func (cbm *CircuitBreakerManager) AddProvider(name string, maxFailures int, resetTimeout time.Duration) {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	cbm.breakers[name] = &CircuitBreaker{
		name:         name,
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        StateClosed,
	}
}

// CanCall checks if a call to the provider is allowed
func (cbm *CircuitBreakerManager) CanCall(provider string) error {
	cbm.mu.RLock()
	breaker, exists := cbm.breakers[provider]
	cbm.mu.RUnlock()

	if !exists {
		// If provider doesn't exist, allow the call
		return nil
	}

	return breaker.canCall()
}

// OnSuccess records a successful call to the provider
func (cbm *CircuitBreakerManager) OnSuccess(provider string) {
	cbm.mu.RLock()
	breaker, exists := cbm.breakers[provider]
	cbm.mu.RUnlock()

	if exists {
		breaker.onSuccess()
	}
}

// OnFailure records a failed call to the provider
func (cbm *CircuitBreakerManager) OnFailure(provider string) {
	cbm.mu.RLock()
	breaker, exists := cbm.breakers[provider]
	cbm.mu.RUnlock()

	if exists {
		breaker.onFailure()
	}
}

// GetProviderState returns the current state of a provider's circuit breaker
func (cbm *CircuitBreakerManager) GetProviderState(provider string) (CircuitState, int, time.Time) {
	cbm.mu.RLock()
	breaker, exists := cbm.breakers[provider]
	cbm.mu.RUnlock()

	if !exists {
		return StateClosed, 0, time.Time{}
	}

	breaker.mu.RLock()
	defer breaker.mu.RUnlock()

	return breaker.state, breaker.failureCount, breaker.lastFailureTime
}

// canCall checks if a call is allowed based on circuit breaker state
func (cb *CircuitBreaker) canCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return nil
	case StateOpen:
		// Check if reset timeout has passed
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = StateHalfOpen
			return nil
		}
		return fmt.Errorf("circuit breaker for %s is OPEN (failed %d times, last failure: %v ago)",
			cb.name, cb.failureCount, time.Since(cb.lastFailureTime).Round(time.Second))
	case StateHalfOpen:
		return nil
	default:
		return errors.New("unknown circuit breaker state")
	}
}

// onSuccess handles a successful call
func (cb *CircuitBreaker) onSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateHalfOpen:
		// Reset to closed state
		cb.state = StateClosed
		cb.failureCount = 0
	case StateClosed:
		// Reset failure count on success
		cb.failureCount = 0
	}
}

// onFailure handles a failed call
func (cb *CircuitBreaker) onFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.failureCount >= cb.maxFailures {
			cb.state = StateOpen
		}
	case StateHalfOpen:
		// Go back to open state
		cb.state = StateOpen
	}
}
