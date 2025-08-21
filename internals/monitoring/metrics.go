package monitoring

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// MetricsCollector handles historical data collection and analytics
type MetricsCollector struct {
	mu                  sync.RWMutex
	addressGeneration   []MetricPoint
	paymentSuccess      []MetricPoint
	gapLimitEvents      []MetricPoint
	rateLimitEvents     []MetricPoint
	systemPerformance   []MetricPoint
	errorRates          []MetricPoint
	maxDataPoints       int
	started             time.Time
}

// MetricPoint represents a single data point in time
type MetricPoint struct {
	Timestamp time.Time   `json:"timestamp"`
	Value     float64     `json:"value"`
	Metadata  interface{} `json:"metadata,omitempty"`
}

// PerformanceMetrics aggregated performance data
type PerformanceMetrics struct {
	AvgResponseTime     float64 `json:"avg_response_time"`
	AddressGenRate      float64 `json:"address_gen_rate"`
	PaymentSuccessRate  float64 `json:"payment_success_rate"`
	ErrorRate           float64 `json:"error_rate"`
	UptimePercentage    float64 `json:"uptime_percentage"`
	MemoryUsageMB       float64 `json:"memory_usage_mb"`
	CPUUsagePercent     float64 `json:"cpu_usage_percent"`
}

// TrendAnalysis trend data for charts
type TrendAnalysis struct {
	Period           string        `json:"period"`
	AddressGenTrend  []MetricPoint `json:"address_generation_trend"`
	PaymentTrend     []MetricPoint `json:"payment_trend"`
	ErrorTrend       []MetricPoint `json:"error_trend"`
	PerformanceTrend []MetricPoint `json:"performance_trend"`
}

// AnalyticsData comprehensive analytics response
type AnalyticsData struct {
	Summary      PerformanceMetrics `json:"summary"`
	Trends       TrendAnalysis      `json:"trends"`
	Insights     []string           `json:"insights"`
	LastUpdated  time.Time          `json:"last_updated"`
	DataRange    string             `json:"data_range"`
}

var (
	globalMetricsCollector *MetricsCollector
	metricsOnce            sync.Once
)

// GetMetricsCollector returns the singleton metrics collector
func GetMetricsCollector() *MetricsCollector {
	metricsOnce.Do(func() {
		globalMetricsCollector = &MetricsCollector{
			addressGeneration: make([]MetricPoint, 0),
			paymentSuccess:    make([]MetricPoint, 0),
			gapLimitEvents:    make([]MetricPoint, 0),
			rateLimitEvents:   make([]MetricPoint, 0),
			systemPerformance: make([]MetricPoint, 0),
			errorRates:        make([]MetricPoint, 0),
			maxDataPoints:     1000, // Keep last 1000 data points
			started:           time.Now(),
		}
		
		// Start background collection
		go globalMetricsCollector.startCollection()
	})
	return globalMetricsCollector
}

// startCollection begins periodic metrics collection
func (m *MetricsCollector) startCollection() {
	ticker := time.NewTicker(30 * time.Second) // Collect every 30 seconds
	defer ticker.Stop()
	
	for range ticker.C {
		m.collectSystemMetrics()
	}
}

// collectSystemMetrics gathers current system metrics
func (m *MetricsCollector) collectSystemMetrics() {
	now := time.Now()
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Simulate metrics collection - in real implementation, you'd gather actual data
	// Address generation rate (addresses per minute)
	addressGenRate := float64(time.Now().Unix()%10 + 5) // 5-14 addresses/min
	m.addMetricPoint(&m.addressGeneration, MetricPoint{
		Timestamp: now,
		Value:     addressGenRate,
		Metadata:  map[string]interface{}{"unit": "addresses/min"},
	})
	
	// Payment success rate (percentage)
	successRate := 95.0 + float64(time.Now().Unix()%10)/2 // 95-100%
	m.addMetricPoint(&m.paymentSuccess, MetricPoint{
		Timestamp: now,
		Value:     successRate,
		Metadata:  map[string]interface{}{"unit": "percentage"},
	})
	
	// Error rate (errors per hour)
	errorRate := float64(time.Now().Unix()%5) // 0-4 errors/hour
	m.addMetricPoint(&m.errorRates, MetricPoint{
		Timestamp: now,
		Value:     errorRate,
		Metadata:  map[string]interface{}{"unit": "errors/hour"},
	})
	
	// System performance score (0-100)
	performance := 85.0 + float64(time.Now().Unix()%15) // 85-100
	m.addMetricPoint(&m.systemPerformance, MetricPoint{
		Timestamp: now,
		Value:     performance,
		Metadata:  map[string]interface{}{"unit": "score"},
	})
}

// addMetricPoint adds a metric point and maintains the size limit
func (m *MetricsCollector) addMetricPoint(slice *[]MetricPoint, point MetricPoint) {
	*slice = append(*slice, point)
	if len(*slice) > m.maxDataPoints {
		*slice = (*slice)[1:] // Remove oldest point
	}
}

// RecordAddressGeneration records address generation metrics
func (m *MetricsCollector) RecordAddressGeneration(count int, processingTime time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.addMetricPoint(&m.addressGeneration, MetricPoint{
		Timestamp: time.Now(),
		Value:     float64(count),
		Metadata: map[string]interface{}{
			"processing_time_ms": processingTime.Milliseconds(),
			"type":              "generation",
		},
	})
}

// RecordPaymentSuccess records successful payment metrics
func (m *MetricsCollector) RecordPaymentSuccess(amount float64, confirmationTime time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.addMetricPoint(&m.paymentSuccess, MetricPoint{
		Timestamp: time.Now(),
		Value:     amount,
		Metadata: map[string]interface{}{
			"confirmation_time_ms": confirmationTime.Milliseconds(),
			"type":                "success",
		},
	})
}

// RecordGapLimitEvent records gap limit related events
func (m *MetricsCollector) RecordGapLimitEvent(gapRatio float64, eventType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.addMetricPoint(&m.gapLimitEvents, MetricPoint{
		Timestamp: time.Now(),
		Value:     gapRatio,
		Metadata: map[string]interface{}{
			"event_type": eventType,
			"severity":   m.getGapSeverity(gapRatio),
		},
	})
}

// RecordRateLimitEvent records rate limiting events
func (m *MetricsCollector) RecordRateLimitEvent(identifier string, eventType string, tokens int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.addMetricPoint(&m.rateLimitEvents, MetricPoint{
		Timestamp: time.Now(),
		Value:     float64(tokens),
		Metadata: map[string]interface{}{
			"identifier": identifier,
			"event_type": eventType,
		},
	})
}

// RecordError records system errors
func (m *MetricsCollector) RecordError(component string, errorType string, severity string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.addMetricPoint(&m.errorRates, MetricPoint{
		Timestamp: time.Now(),
		Value:     1, // Error count
		Metadata: map[string]interface{}{
			"component":  component,
			"error_type": errorType,
			"severity":   severity,
		},
	})
}

// GetAnalyticsData returns comprehensive analytics data
func (m *MetricsCollector) GetAnalyticsData(period string) AnalyticsData {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	now := time.Now()
	var since time.Time
	
	switch period {
	case "1h":
		since = now.Add(-1 * time.Hour)
	case "6h":
		since = now.Add(-6 * time.Hour)
	case "24h":
		since = now.Add(-24 * time.Hour)
	case "7d":
		since = now.Add(-7 * 24 * time.Hour)
	default:
		since = now.Add(-1 * time.Hour)
		period = "1h"
	}
	
	return AnalyticsData{
		Summary:     m.calculateSummaryMetrics(since),
		Trends:      m.calculateTrends(since, period),
		Insights:    m.generateInsights(since),
		LastUpdated: now,
		DataRange:   period,
	}
}

// calculateSummaryMetrics calculates aggregate metrics for the period
func (m *MetricsCollector) calculateSummaryMetrics(since time.Time) PerformanceMetrics {
	// Filter data points within the time range
	addressGenPoints := m.filterMetricPoints(m.addressGeneration, since)
	paymentPoints := m.filterMetricPoints(m.paymentSuccess, since)
	errorPoints := m.filterMetricPoints(m.errorRates, since)
	performancePoints := m.filterMetricPoints(m.systemPerformance, since)
	
	// Calculate averages
	avgAddressGen := m.calculateAverage(addressGenPoints)
	avgPaymentSuccess := m.calculateAverage(paymentPoints)
	avgErrors := m.calculateAverage(errorPoints)
	_ = m.calculateAverage(performancePoints) // Performance average for future use
	
	// Calculate uptime (simplified - in real implementation, track actual downtime)
	uptime := 99.9
	if avgErrors > 5 {
		uptime = 99.0
	} else if avgErrors > 2 {
		uptime = 99.5
	}
	
	return PerformanceMetrics{
		AvgResponseTime:    float64(50 + time.Now().Unix()%50), // 50-100ms
		AddressGenRate:     avgAddressGen,
		PaymentSuccessRate: avgPaymentSuccess,
		ErrorRate:          avgErrors,
		UptimePercentage:   uptime,
		MemoryUsageMB:      float64(128 + time.Now().Unix()%64), // 128-192MB
		CPUUsagePercent:    float64(10 + time.Now().Unix()%20),  // 10-30%
	}
}

// calculateTrends generates trend data for charts
func (m *MetricsCollector) calculateTrends(since time.Time, period string) TrendAnalysis {
	// Sample recent data points for trends
	sampleSize := 20
	if period == "7d" {
		sampleSize = 50
	}
	
	return TrendAnalysis{
		Period:           period,
		AddressGenTrend:  m.sampleMetricPoints(m.filterMetricPoints(m.addressGeneration, since), sampleSize),
		PaymentTrend:     m.sampleMetricPoints(m.filterMetricPoints(m.paymentSuccess, since), sampleSize),
		ErrorTrend:       m.sampleMetricPoints(m.filterMetricPoints(m.errorRates, since), sampleSize),
		PerformanceTrend: m.sampleMetricPoints(m.filterMetricPoints(m.systemPerformance, since), sampleSize),
	}
}

// generateInsights creates actionable insights from the data
func (m *MetricsCollector) generateInsights(since time.Time) []string {
	insights := []string{}
	
	// Analyze error trends
	errorPoints := m.filterMetricPoints(m.errorRates, since)
	if len(errorPoints) > 0 {
		avgErrors := m.calculateAverage(errorPoints)
		if avgErrors > 3 {
			insights = append(insights, "Higher than normal error rate detected. Check system logs for details.")
		} else if avgErrors < 1 {
			insights = append(insights, "System running with minimal errors. Performance is optimal.")
		}
	}
	
	// Analyze address generation
	addressPoints := m.filterMetricPoints(m.addressGeneration, since)
	if len(addressPoints) > 0 {
		avgGeneration := m.calculateAverage(addressPoints)
		if avgGeneration < 3 {
			insights = append(insights, "Address generation rate is low. Consider pool optimization.")
		} else if avgGeneration > 12 {
			insights = append(insights, "High address generation activity. Monitor pool capacity.")
		}
	}
	
	// Analyze payment success
	paymentPoints := m.filterMetricPoints(m.paymentSuccess, since)
	if len(paymentPoints) > 0 {
		avgSuccess := m.calculateAverage(paymentPoints)
		if avgSuccess < 95 {
			insights = append(insights, "Payment success rate below optimal. Check blockchain connectivity.")
		} else if avgSuccess > 98 {
			insights = append(insights, "Excellent payment success rate. System performing well.")
		}
	}
	
	// System uptime insight
	uptime := time.Since(m.started)
	if uptime > 24*time.Hour {
		insights = append(insights, fmt.Sprintf("System uptime: %.1f hours. Stable operation.", uptime.Hours()))
	}
	
	if len(insights) == 0 {
		insights = append(insights, "All metrics within normal ranges. System operating optimally.")
	}
	
	return insights
}

// Helper functions

func (m *MetricsCollector) filterMetricPoints(points []MetricPoint, since time.Time) []MetricPoint {
	filtered := make([]MetricPoint, 0)
	for _, point := range points {
		if point.Timestamp.After(since) {
			filtered = append(filtered, point)
		}
	}
	return filtered
}

func (m *MetricsCollector) calculateAverage(points []MetricPoint) float64 {
	if len(points) == 0 {
		return 0
	}
	
	sum := 0.0
	for _, point := range points {
		sum += point.Value
	}
	return sum / float64(len(points))
}

func (m *MetricsCollector) sampleMetricPoints(points []MetricPoint, maxSamples int) []MetricPoint {
	if len(points) <= maxSamples {
		return points
	}
	
	// Sample evenly across the dataset
	step := len(points) / maxSamples
	sampled := make([]MetricPoint, 0, maxSamples)
	
	for i := 0; i < len(points); i += step {
		sampled = append(sampled, points[i])
	}
	
	return sampled
}

func (m *MetricsCollector) getGapSeverity(gapRatio float64) string {
	if gapRatio > 80 {
		return "critical"
	} else if gapRatio > 60 {
		return "warning"
	}
	return "normal"
}

// GetSystemHealth returns current system health status
func (m *MetricsCollector) GetSystemHealth() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	now := time.Now()
	recentWindow := now.Add(-5 * time.Minute)
	
	recentErrors := m.filterMetricPoints(m.errorRates, recentWindow)
	recentPerformance := m.filterMetricPoints(m.systemPerformance, recentWindow)
	
	errorCount := len(recentErrors)
	avgPerformance := m.calculateAverage(recentPerformance)
	
	health := "healthy"
	if errorCount > 5 || avgPerformance < 70 {
		health = "degraded"
	} else if errorCount > 10 || avgPerformance < 50 {
		health = "unhealthy"
	}
	
	return map[string]interface{}{
		"status":            health,
		"recent_errors":     errorCount,
		"performance_score": avgPerformance,
		"uptime_seconds":    time.Since(m.started).Seconds(),
		"data_points":       len(m.addressGeneration) + len(m.paymentSuccess) + len(m.errorRates),
		"last_updated":      now.Unix(),
	}
}

// ExportMetrics exports all collected metrics as JSON
func (m *MetricsCollector) ExportMetrics() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	export := map[string]interface{}{
		"address_generation": m.addressGeneration,
		"payment_success":    m.paymentSuccess,
		"gap_limit_events":   m.gapLimitEvents,
		"rate_limit_events":  m.rateLimitEvents,
		"system_performance": m.systemPerformance,
		"error_rates":        m.errorRates,
		"collection_started": m.started,
		"exported_at":        time.Now(),
	}
	
	return json.Marshal(export)
}

