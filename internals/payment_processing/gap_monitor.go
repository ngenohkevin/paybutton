package payment_processing

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// GapLimitMonitor monitors and alerts on gap limit issues
type GapLimitMonitor struct {
	mu                  sync.RWMutex
	totalAddresses      int
	paidAddresses       int
	unpaidAddresses     int
	recentErrors        []GapLimitError
	warningThreshold    float64 // Percentage of gap limit before warning
	criticalThreshold   float64 // Percentage of gap limit before critical alert
	lastAlert           time.Time
	alertCooldown       time.Duration
	consecutiveFailures int
	maxGapLimit         int // Blockonomics default is usually 20
}

// GapLimitError represents a gap limit error occurrence
type GapLimitError struct {
	Timestamp time.Time `json:"timestamp"`
	Email     string    `json:"email"`
	Message   string    `json:"message"`
}

var (
	gapMonitor     *GapLimitMonitor
	gapMonitorOnce sync.Once
)

// InitializeGapMonitor creates and initializes the gap monitor
func InitializeGapMonitor() *GapLimitMonitor {
	gapMonitorOnce.Do(func() {
		gapMonitor = &GapLimitMonitor{
			recentErrors:      make([]GapLimitError, 0),
			warningThreshold:  0.7,  // Alert at 70% of gap limit
			criticalThreshold: 0.85, // Critical at 85% of gap limit
			alertCooldown:     15 * time.Minute,
			maxGapLimit:       20, // Blockonomics default
		}

		// Start monitoring goroutine
		go gapMonitor.monitor()
	})
	return gapMonitor
}

// GetGapMonitor returns the singleton gap monitor instance
func GetGapMonitor() *GapLimitMonitor {
	if gapMonitor == nil {
		return InitializeGapMonitor()
	}
	return gapMonitor
}

// RecordAddressGeneration records a successful address generation
func (m *GapLimitMonitor) RecordAddressGeneration() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalAddresses++
	m.unpaidAddresses++
	m.consecutiveFailures = 0 // Reset failure counter on success

	// Check if we're approaching the limit
	gapRatio := float64(m.unpaidAddresses) / float64(m.maxGapLimit)

	// CRITICAL FIX: Force address reuse when approaching gap limit
	if m.unpaidAddresses >= 15 {
		log.Printf("⚠️ GAP LIMIT PREVENTION: %d unpaid addresses - MUST reuse addresses!", m.unpaidAddresses)
	}

	if gapRatio >= m.criticalThreshold {
		m.sendAlert("CRITICAL", fmt.Sprintf("Gap limit critical: %d/%d unpaid addresses (%.0f%%)",
			m.unpaidAddresses, m.maxGapLimit, gapRatio*100))
	} else if gapRatio >= m.warningThreshold {
		m.sendAlert("WARNING", fmt.Sprintf("Gap limit warning: %d/%d unpaid addresses (%.0f%%)",
			m.unpaidAddresses, m.maxGapLimit, gapRatio*100))
	}
}

// RecordPayment records when an address receives payment
func (m *GapLimitMonitor) RecordPayment(address string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.paidAddresses++
	if m.unpaidAddresses > 0 {
		m.unpaidAddresses--
	}

	log.Printf("Payment recorded for %s. Gap status: %d unpaid, %d paid",
		address, m.unpaidAddresses, m.paidAddresses)
}

// RecordGapLimitError records a gap limit error
func (m *GapLimitMonitor) RecordGapLimitError(email, errorMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.consecutiveFailures++

	// Add to recent errors
	m.recentErrors = append(m.recentErrors, GapLimitError{
		Timestamp: time.Now(),
		Email:     email,
		Message:   errorMsg,
	})

	// Keep only last 100 errors
	if len(m.recentErrors) > 100 {
		m.recentErrors = m.recentErrors[len(m.recentErrors)-100:]
	}

	// Send alert based on consecutive failures
	alertMsg := ""
	if m.consecutiveFailures >= 5 {
		alertMsg = fmt.Sprintf("CRITICAL: %d consecutive gap limit errors! Switching to fallback mode.", m.consecutiveFailures)
	} else if m.consecutiveFailures >= 3 {
		alertMsg = fmt.Sprintf("WARNING: %d consecutive gap limit errors", m.consecutiveFailures)
	}

	if alertMsg != "" {
		m.sendAlert("GAP_LIMIT", alertMsg)
	}
}

// ShouldUseFallback determines if we should use fallback addresses
func (m *GapLimitMonitor) ShouldUseFallback() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Use fallback if:
	// 1. Too many consecutive failures
	if m.consecutiveFailures >= 3 {
		return true
	}

	// 2. Gap ratio is critical - use dynamic threshold based on recent activity
	// If we have many recent errors, be more conservative
	dynamicThreshold := m.criticalThreshold
	if len(m.recentErrors) > 3 {
		dynamicThreshold = 0.7 // Lower threshold when experiencing issues
	}
	
	gapRatio := float64(m.unpaidAddresses) / float64(m.maxGapLimit)
	if gapRatio >= dynamicThreshold {
		return true
	}

	// 3. Recent error rate is high
	now := time.Now()
	recentErrorCount := 0
	for _, err := range m.recentErrors {
		if now.Sub(err.Timestamp) < 5*time.Minute {
			recentErrorCount++
		}
	}
	if recentErrorCount >= 5 {
		return true
	}

	return false
}

// GetStats returns current monitoring statistics
func (m *GapLimitMonitor) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	gapRatio := float64(m.unpaidAddresses) / float64(m.maxGapLimit)

	return map[string]interface{}{
		"total_addresses":      m.totalAddresses,
		"paid_addresses":       m.paidAddresses,
		"unpaid_addresses":     m.unpaidAddresses,
		"gap_ratio":            fmt.Sprintf("%.2f%%", gapRatio*100),
		"consecutive_failures": m.consecutiveFailures,
		"recent_errors":        len(m.recentErrors),
		"should_use_fallback":  m.ShouldUseFallback(),
		"max_gap_limit":        m.maxGapLimit,
	}
}

// sendAlert sends an alert to Telegram
func (m *GapLimitMonitor) sendAlert(level, message string) {
	now := time.Now()
	if now.Sub(m.lastAlert) < m.alertCooldown {
		return // Skip if in cooldown
	}

	m.lastAlert = now

	// Log the alert
	alertMsg := fmt.Sprintf("🚨 [%s ALERT] %s", level, message)
	log.Printf(alertMsg)

	// Note: To send to Telegram, pass the bot instance from your main server
	// or implement a global bot getter function
}

// monitor runs periodic monitoring tasks
func (m *GapLimitMonitor) monitor() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.RLock()
		stats := m.GetStats()
		m.mu.RUnlock()

		// Log current status
		log.Printf("Gap Monitor Status: %v", stats)

		// Auto-recover from fallback mode if conditions improve
		// Also check for stale unpaid addresses that might have been paid
		if m.unpaidAddresses > 10 {
			// Audit actual unpaid addresses - some may have been paid but not recorded
			m.auditUnpaidAddresses()
		}
		
		if m.consecutiveFailures > 0 && m.unpaidAddresses < int(float64(m.maxGapLimit)*0.5) {
			m.mu.Lock()
			m.consecutiveFailures = 0
			m.mu.Unlock()
			log.Printf("Gap limit conditions improved, resetting failure counter")
		}
	}
}

// auditUnpaidAddresses checks if "unpaid" addresses have actually been paid
func (m *GapLimitMonitor) auditUnpaidAddresses() {
	addressPool := GetAddressPool()
	if addressPool == nil {
		return
	}
	
	// Get pool stats to cross-reference
	poolStats := addressPool.GetStats()
	actualUnpaid := poolStats.TotalGenerated - poolStats.TotalUsed
	
	if actualUnpaid < m.unpaidAddresses {
		m.mu.Lock()
		oldCount := m.unpaidAddresses
		m.unpaidAddresses = actualUnpaid
		m.mu.Unlock()
		log.Printf("Gap monitor audit: Corrected unpaid count from %d to %d", oldCount, actualUnpaid)
	}
}

// ResetUnpaidCount manually resets the unpaid count (for admin use)
func (m *GapLimitMonitor) ResetUnpaidCount(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.unpaidAddresses = count
	m.consecutiveFailures = 0
	log.Printf("Manually reset unpaid count to %d", count)
}

// GetRecentErrors returns the recent error history
func (m *GapLimitMonitor) GetRecentErrors() []GapLimitError {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to avoid concurrent access issues
	errors := make([]GapLimitError, len(m.recentErrors))
	copy(errors, m.recentErrors)
	return errors
}

// ClearRecentErrors clears the recent error history (for admin use)
func (m *GapLimitMonitor) ClearRecentErrors() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recentErrors = make([]GapLimitError, 0)
	log.Printf("Recent error history cleared")
}

// UpdateThresholds updates the warning and critical thresholds (for admin use)
func (m *GapLimitMonitor) UpdateThresholds(warning, critical float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if warning >= 0 && warning <= 1 {
		m.warningThreshold = warning
	}
	if critical >= 0 && critical <= 1 {
		m.criticalThreshold = critical
	}

	log.Printf("Updated thresholds: warning=%.2f, critical=%.2f", m.warningThreshold, m.criticalThreshold)
}

// UpdateMaxGapLimit updates the maximum gap limit (for admin use)
func (m *GapLimitMonitor) UpdateMaxGapLimit(limit int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if limit > 0 && limit <= 100 {
		m.maxGapLimit = limit
		log.Printf("Updated max gap limit to %d", limit)
	}
}

// GetThresholds returns current warning and critical thresholds
func (m *GapLimitMonitor) GetThresholds() (float64, float64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.warningThreshold, m.criticalThreshold
}
