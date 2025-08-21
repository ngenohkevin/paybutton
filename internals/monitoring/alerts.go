package monitoring

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// AlertSeverity represents the severity level of an alert
type AlertSeverity string

const (
	SeverityInfo     AlertSeverity = "info"
	SeverityWarning  AlertSeverity = "warning"
	SeverityCritical AlertSeverity = "critical"
)

// AlertStatus represents the current status of an alert
type AlertStatus string

const (
	StatusActive       AlertStatus = "active"
	StatusAcknowledged AlertStatus = "acknowledged"
	StatusResolved     AlertStatus = "resolved"
)

// Alert represents a system alert
type Alert struct {
	ID         string                 `json:"id"`
	Title      string                 `json:"title"`
	Message    string                 `json:"message"`
	Severity   AlertSeverity          `json:"severity"`
	Status     AlertStatus            `json:"status"`
	Component  string                 `json:"component"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	ResolvedAt *time.Time             `json:"resolved_at,omitempty"`
	AckedBy    string                 `json:"acked_by,omitempty"`
	AckedAt    *time.Time             `json:"acked_at,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// AlertRule defines when to trigger an alert
type AlertRule struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Component   string        `json:"component"`
	Metric      string        `json:"metric"`
	Operator    string        `json:"operator"` // >, <, >=, <=, ==, !=
	Threshold   float64       `json:"threshold"`
	Duration    time.Duration `json:"duration"`
	Severity    AlertSeverity `json:"severity"`
	Enabled     bool          `json:"enabled"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// NotificationChannel represents a way to send alerts
type NotificationChannel struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Type     string                 `json:"type"` // email, webhook, telegram
	Config   map[string]interface{} `json:"config"`
	Enabled  bool                   `json:"enabled"`
	TestMode bool                   `json:"test_mode"`
}

// AlertManager manages the alert system
type AlertManager struct {
	mu                  sync.RWMutex
	alerts              map[string]*Alert
	rules               map[string]*AlertRule
	channels            map[string]*NotificationChannel
	evaluationTicker    *time.Ticker
	evaluationInterval  time.Duration
	maxActiveAlerts     int
	alertHistory        []*Alert
	maxHistorySize      int
	lastMetrics         map[string]float64
	ruleViolationCounts map[string]int
}

// NewAlertManager creates a new alert manager
func NewAlertManager() *AlertManager {
	am := &AlertManager{
		alerts:              make(map[string]*Alert),
		rules:               make(map[string]*AlertRule),
		channels:            make(map[string]*NotificationChannel),
		evaluationInterval:  30 * time.Second,
		maxActiveAlerts:     100,
		alertHistory:        make([]*Alert, 0),
		maxHistorySize:      1000,
		lastMetrics:         make(map[string]float64),
		ruleViolationCounts: make(map[string]int),
	}

	// Initialize default rules
	am.initializeDefaultRules()

	// Initialize default notification channels
	am.initializeDefaultChannels()

	// Start evaluation ticker
	am.startEvaluation()

	return am
}

// initializeDefaultRules sets up default alert rules
func (am *AlertManager) initializeDefaultRules() {
	defaultRules := []*AlertRule{
		{
			ID:          "pool-low-addresses",
			Name:        "Address Pool Low",
			Description: "Alert when available address count is below threshold",
			Component:   "address_pool",
			Metric:      "available_count",
			Operator:    "<",
			Threshold:   10,
			Duration:    5 * time.Minute,
			Severity:    SeverityWarning,
			Enabled:     true,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			ID:          "pool-critical-addresses",
			Name:        "Address Pool Critical",
			Description: "Critical alert when available address count is critically low",
			Component:   "address_pool",
			Metric:      "available_count",
			Operator:    "<",
			Threshold:   5,
			Duration:    2 * time.Minute,
			Severity:    SeverityCritical,
			Enabled:     true,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			ID:          "gap-limit-warning",
			Name:        "Gap Limit Warning",
			Description: "Alert when gap limit ratio exceeds warning threshold",
			Component:   "gap_monitor",
			Metric:      "gap_ratio",
			Operator:    ">",
			Threshold:   0.7,
			Duration:    10 * time.Minute,
			Severity:    SeverityWarning,
			Enabled:     true,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			ID:          "gap-limit-critical",
			Name:        "Gap Limit Critical",
			Description: "Critical alert when gap limit ratio is critical",
			Component:   "gap_monitor",
			Metric:      "gap_ratio",
			Operator:    ">",
			Threshold:   0.9,
			Duration:    5 * time.Minute,
			Severity:    SeverityCritical,
			Enabled:     true,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			ID:          "high-rate-limit-violations",
			Name:        "High Rate Limit Violations",
			Description: "Alert when rate limit violations are high",
			Component:   "rate_limiter",
			Metric:      "total_violations",
			Operator:    ">",
			Threshold:   50,
			Duration:    5 * time.Minute,
			Severity:    SeverityWarning,
			Enabled:     true,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
	}

	for _, rule := range defaultRules {
		am.rules[rule.ID] = rule
	}
}

// initializeDefaultChannels sets up default notification channels
func (am *AlertManager) initializeDefaultChannels() {
	defaultChannels := []*NotificationChannel{
		{
			ID:      "console",
			Name:    "Console Logging",
			Type:    "console",
			Config:  map[string]interface{}{},
			Enabled: true,
		},
		{
			ID:   "webhook",
			Name: "Webhook Notifications",
			Type: "webhook",
			Config: map[string]interface{}{
				"url":     "",
				"method":  "POST",
				"headers": map[string]string{"Content-Type": "application/json"},
			},
			Enabled: false,
		},
		{
			ID:   "email",
			Name: "Email Notifications",
			Type: "email",
			Config: map[string]interface{}{
				"smtp_host":     "",
				"smtp_port":     587,
				"smtp_username": "",
				"smtp_password": "",
				"from_email":    "",
				"to_emails":     []string{},
			},
			Enabled: false,
		},
	}

	for _, channel := range defaultChannels {
		am.channels[channel.ID] = channel
	}
}

// startEvaluation starts the periodic rule evaluation
func (am *AlertManager) startEvaluation() {
	am.evaluationTicker = time.NewTicker(am.evaluationInterval)
	go func() {
		for range am.evaluationTicker.C {
			am.evaluateRules()
		}
	}()
}

// TriggerAlert manually triggers an alert
func (am *AlertManager) TriggerAlert(title, message, component string, severity AlertSeverity, metadata map[string]interface{}) string {
	am.mu.Lock()
	defer am.mu.Unlock()

	alertID := fmt.Sprintf("alert_%d", time.Now().UnixNano())
	alert := &Alert{
		ID:        alertID,
		Title:     title,
		Message:   message,
		Severity:  severity,
		Status:    StatusActive,
		Component: component,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  metadata,
	}

	am.alerts[alertID] = alert
	am.addToHistory(alert)

	// Send notifications
	go am.sendNotifications(alert)

	return alertID
}

// AcknowledgeAlert acknowledges an alert
func (am *AlertManager) AcknowledgeAlert(alertID, ackedBy string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	alert, exists := am.alerts[alertID]
	if !exists {
		return fmt.Errorf("alert not found: %s", alertID)
	}

	if alert.Status == StatusResolved {
		return fmt.Errorf("cannot acknowledge resolved alert")
	}

	now := time.Now()
	alert.Status = StatusAcknowledged
	alert.AckedBy = ackedBy
	alert.AckedAt = &now
	alert.UpdatedAt = now

	return nil
}

// ResolveAlert resolves an alert
func (am *AlertManager) ResolveAlert(alertID string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	alert, exists := am.alerts[alertID]
	if !exists {
		return fmt.Errorf("alert not found: %s", alertID)
	}

	now := time.Now()
	alert.Status = StatusResolved
	alert.ResolvedAt = &now
	alert.UpdatedAt = now

	// Remove from active alerts
	delete(am.alerts, alertID)

	return nil
}

// GetActiveAlerts returns all active alerts
func (am *AlertManager) GetActiveAlerts() []*Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	alerts := make([]*Alert, 0, len(am.alerts))
	for _, alert := range am.alerts {
		alerts = append(alerts, alert)
	}

	return alerts
}

// GetAlertHistory returns alert history
func (am *AlertManager) GetAlertHistory(limit int) []*Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	if limit <= 0 || limit > len(am.alertHistory) {
		limit = len(am.alertHistory)
	}

	// Return most recent alerts first
	result := make([]*Alert, limit)
	for i := 0; i < limit; i++ {
		result[i] = am.alertHistory[len(am.alertHistory)-1-i]
	}

	return result
}

// GetRules returns all alert rules
func (am *AlertManager) GetRules() []*AlertRule {
	am.mu.RLock()
	defer am.mu.RUnlock()

	rules := make([]*AlertRule, 0, len(am.rules))
	for _, rule := range am.rules {
		rules = append(rules, rule)
	}

	return rules
}

// GetChannels returns all notification channels
func (am *AlertManager) GetChannels() []*NotificationChannel {
	am.mu.RLock()
	defer am.mu.RUnlock()

	channels := make([]*NotificationChannel, 0, len(am.channels))
	for _, channel := range am.channels {
		channels = append(channels, channel)
	}

	return channels
}

// UpdateRule updates an alert rule
func (am *AlertManager) UpdateRule(rule *AlertRule) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	rule.UpdatedAt = time.Now()
	am.rules[rule.ID] = rule

	return nil
}

// UpdateChannel updates a notification channel
func (am *AlertManager) UpdateChannel(channel *NotificationChannel) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	am.channels[channel.ID] = channel

	return nil
}

// TestChannel tests a notification channel
func (am *AlertManager) TestChannel(channelID string) error {
	am.mu.RLock()
	channel, exists := am.channels[channelID]
	am.mu.RUnlock()

	if !exists {
		return fmt.Errorf("channel not found: %s", channelID)
	}

	testAlert := &Alert{
		ID:        "test_alert",
		Title:     "Test Alert",
		Message:   "This is a test alert to verify notification channel configuration",
		Severity:  SeverityInfo,
		Status:    StatusActive,
		Component: "alert_system",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	return am.sendToChannel(testAlert, channel)
}

// GetStats returns alert system statistics
func (am *AlertManager) GetStats() map[string]interface{} {
	am.mu.RLock()
	defer am.mu.RUnlock()

	activeByStatus := make(map[string]int)
	activeBySeverity := make(map[string]int)

	for _, alert := range am.alerts {
		activeByStatus[string(alert.Status)]++
		activeBySeverity[string(alert.Severity)]++
	}

	enabledRules := 0
	for _, rule := range am.rules {
		if rule.Enabled {
			enabledRules++
		}
	}

	enabledChannels := 0
	for _, channel := range am.channels {
		if channel.Enabled {
			enabledChannels++
		}
	}

	return map[string]interface{}{
		"active_alerts":      len(am.alerts),
		"total_rules":        len(am.rules),
		"enabled_rules":      enabledRules,
		"enabled_channels":   enabledChannels,
		"alerts_by_status":   activeByStatus,
		"alerts_by_severity": activeBySeverity,
		"history_size":       len(am.alertHistory),
	}
}

// evaluateRules evaluates all enabled rules against current metrics
func (am *AlertManager) evaluateRules() {
	// This would be called periodically to check metrics against rules
	// For now, we'll implement basic checks

	// Note: In a real implementation, you'd get actual metrics from your monitoring system
	// For this example, we'll use placeholder logic
}

// addToHistory adds an alert to the history
func (am *AlertManager) addToHistory(alert *Alert) {
	am.alertHistory = append(am.alertHistory, alert)

	// Trim history if it gets too large
	if len(am.alertHistory) > am.maxHistorySize {
		am.alertHistory = am.alertHistory[len(am.alertHistory)-am.maxHistorySize:]
	}
}

// sendNotifications sends alert notifications to all enabled channels
func (am *AlertManager) sendNotifications(alert *Alert) {
	am.mu.RLock()
	channels := make([]*NotificationChannel, 0, len(am.channels))
	for _, channel := range am.channels {
		if channel.Enabled {
			channels = append(channels, channel)
		}
	}
	am.mu.RUnlock()

	for _, channel := range channels {
		if err := am.sendToChannel(alert, channel); err != nil {
			log.Printf("Failed to send alert to channel %s: %v", channel.Name, err)
		}
	}
}

// sendToChannel sends an alert to a specific notification channel
func (am *AlertManager) sendToChannel(alert *Alert, channel *NotificationChannel) error {
	switch channel.Type {
	case "console":
		return am.sendToConsole(alert)
	case "webhook":
		return am.sendToWebhook(alert, channel)
	case "email":
		return am.sendToEmail(alert, channel)
	default:
		return fmt.Errorf("unsupported channel type: %s", channel.Type)
	}
}

// sendToConsole logs the alert to console
func (am *AlertManager) sendToConsole(alert *Alert) error {
	log.Printf("[ALERT][%s] %s: %s", alert.Severity, alert.Title, alert.Message)
	return nil
}

// sendToWebhook sends alert to webhook
func (am *AlertManager) sendToWebhook(alert *Alert, channel *NotificationChannel) error {
	url, ok := channel.Config["url"].(string)
	if !ok || url == "" {
		return fmt.Errorf("webhook URL not configured")
	}

	payload := map[string]interface{}{
		"alert":     alert,
		"timestamp": time.Now().Unix(),
		"source":    "paybutton-admin",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add custom headers if configured
	if headers, ok := channel.Config["headers"].(map[string]string); ok {
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// sendToEmail sends alert via email (placeholder - would need SMTP implementation)
func (am *AlertManager) sendToEmail(alert *Alert, channel *NotificationChannel) error {
	// Placeholder for email implementation
	// In a real implementation, you'd use SMTP to send emails
	log.Printf("Email alert: [%s] %s - %s", alert.Severity, alert.Title, alert.Message)
	return nil
}

// Global alert manager instance
var globalAlertManager *AlertManager

// InitializeAlertManager initializes the global alert manager
func InitializeAlertManager() {
	globalAlertManager = NewAlertManager()
}

// GetAlertManager returns the global alert manager instance
func GetAlertManager() *AlertManager {
	if globalAlertManager == nil {
		InitializeAlertManager()
	}
	return globalAlertManager
}
