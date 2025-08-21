package config

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// ConfigManager manages hot-reloadable configuration
type ConfigManager struct {
	mu              sync.RWMutex
	currentConfig   *SystemConfig
	history         []ConfigChange
	maxHistorySize  int
	changeListeners map[string]func(*SystemConfig)
}

// SystemConfig represents the complete system configuration
type SystemConfig struct {
	Timestamp      time.Time       `json:"timestamp"`
	Version        string          `json:"version"`
	AddressPool    PoolConfig      `json:"address_pool"`
	GapMonitor     GapConfig       `json:"gap_monitor"`
	RateLimiter    RateLimitConfig `json:"rate_limiter"`
	WebSocket      WebSocketConfig `json:"websocket"`
	AdminDashboard AdminConfig     `json:"admin_dashboard"`
}

// PoolConfig represents address pool configuration
type PoolConfig struct {
	MinPoolSize     int `json:"min_pool_size" validate:"min=1,max=100"`
	MaxPoolSize     int `json:"max_pool_size" validate:"min=5,max=1000"`
	RefillThreshold int `json:"refill_threshold" validate:"min=1"`
	RefillBatchSize int `json:"refill_batch_size" validate:"min=1,max=50"`
	CleanupInterval int `json:"cleanup_interval_minutes" validate:"min=5,max=1440"`
}

// GapConfig represents gap monitor configuration
type GapConfig struct {
	MaxGapLimit              int     `json:"max_gap_limit" validate:"min=10,max=100"`
	WarningThreshold         float64 `json:"warning_threshold" validate:"min=0,max=1"`
	CriticalThreshold        float64 `json:"critical_threshold" validate:"min=0,max=1"`
	ConsecutiveFailThreshold int     `json:"consecutive_fail_threshold" validate:"min=1,max=20"`
	ErrorHistorySize         int     `json:"error_history_size" validate:"min=10,max=1000"`
}

// RateLimitConfig represents rate limiter configuration
type RateLimitConfig struct {
	GlobalMaxTokens  int `json:"global_max_tokens" validate:"min=50,max=1000"`
	GlobalRefillRate int `json:"global_refill_rate" validate:"min=1,max=100"`
	IPMaxTokens      int `json:"ip_max_tokens" validate:"min=1,max=50"`
	IPRefillRate     int `json:"ip_refill_rate" validate:"min=1,max=20"`
	EmailMaxTokens   int `json:"email_max_tokens" validate:"min=1,max=20"`
	EmailRefillRate  int `json:"email_refill_rate" validate:"min=1,max=10"`
	CleanupInterval  int `json:"cleanup_interval_minutes" validate:"min=5,max=1440"`
}

// WebSocketConfig represents WebSocket configuration
type WebSocketConfig struct {
	ReadBufferSize  int `json:"read_buffer_size" validate:"min=512,max=8192"`
	WriteBufferSize int `json:"write_buffer_size" validate:"min=512,max=8192"`
	PingInterval    int `json:"ping_interval_seconds" validate:"min=10,max=300"`
	PongTimeout     int `json:"pong_timeout_seconds" validate:"min=5,max=120"`
	MaxConnections  int `json:"max_connections" validate:"min=10,max=10000"`
}

// AdminConfig represents admin dashboard configuration
type AdminConfig struct {
	SessionTimeout   int  `json:"session_timeout_hours" validate:"min=1,max=168"`
	RefreshInterval  int  `json:"refresh_interval_seconds" validate:"min=5,max=300"`
	MaxLogEntries    int  `json:"max_log_entries" validate:"min=100,max=10000"`
	EnableMetrics    bool `json:"enable_metrics"`
	EnableAlerts     bool `json:"enable_alerts"`
	MetricsRetention int  `json:"metrics_retention_hours" validate:"min=1,max=8760"`
}

// ConfigChange represents a configuration change entry
type ConfigChange struct {
	ID          string        `json:"id"`
	Timestamp   time.Time     `json:"timestamp"`
	ChangedBy   string        `json:"changed_by"`
	Section     string        `json:"section"`
	Changes     []FieldChange `json:"changes"`
	PrevConfig  *SystemConfig `json:"prev_config,omitempty"`
	Description string        `json:"description"`
	Success     bool          `json:"success"`
	ErrorMsg    string        `json:"error_msg,omitempty"`
}

// FieldChange represents a single field change
type FieldChange struct {
	Field    string      `json:"field"`
	OldValue interface{} `json:"old_value"`
	NewValue interface{} `json:"new_value"`
}

var (
	configManager     *ConfigManager
	configManagerOnce sync.Once
)

// GetConfigManager returns the singleton configuration manager
func GetConfigManager() *ConfigManager {
	configManagerOnce.Do(func() {
		configManager = &ConfigManager{
			currentConfig:   getDefaultConfig(),
			history:         []ConfigChange{},
			maxHistorySize:  100,
			changeListeners: make(map[string]func(*SystemConfig)),
		}
		log.Printf("Configuration manager initialized with default settings")
	})
	return configManager
}

// getDefaultConfig returns the default system configuration
func getDefaultConfig() *SystemConfig {
	return &SystemConfig{
		Timestamp: time.Now(),
		Version:   "1.0.0",
		AddressPool: PoolConfig{
			MinPoolSize:     10,
			MaxPoolSize:     100,
			RefillThreshold: 5,
			RefillBatchSize: 20,
			CleanupInterval: 60,
		},
		GapMonitor: GapConfig{
			MaxGapLimit:              20,
			WarningThreshold:         0.7,
			CriticalThreshold:        0.9,
			ConsecutiveFailThreshold: 5,
			ErrorHistorySize:         100,
		},
		RateLimiter: RateLimitConfig{
			GlobalMaxTokens:  500,
			GlobalRefillRate: 50,
			IPMaxTokens:      10,
			IPRefillRate:     5,
			EmailMaxTokens:   5,
			EmailRefillRate:  2,
			CleanupInterval:  30,
		},
		WebSocket: WebSocketConfig{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			PingInterval:    30,
			PongTimeout:     60,
			MaxConnections:  1000,
		},
		AdminDashboard: AdminConfig{
			SessionTimeout:   24,
			RefreshInterval:  10,
			MaxLogEntries:    1000,
			EnableMetrics:    true,
			EnableAlerts:     true,
			MetricsRetention: 168, // 7 days
		},
	}
}

// GetCurrentConfig returns a copy of the current configuration
func (cm *ConfigManager) GetCurrentConfig() *SystemConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Return a deep copy to prevent external modifications
	configJSON, _ := json.Marshal(cm.currentConfig)
	var configCopy SystemConfig
	json.Unmarshal(configJSON, &configCopy)

	return &configCopy
}

// UpdateConfig updates the configuration with validation
func (cm *ConfigManager) UpdateConfig(newConfig *SystemConfig, changedBy, description string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Validate the new configuration
	if err := cm.validateConfig(newConfig); err != nil {
		cm.recordFailedChange(changedBy, "all", description, err)
		return fmt.Errorf("validation failed: %w", err)
	}

	// Record the change
	changes := cm.detectChanges(cm.currentConfig, newConfig)
	changeID := fmt.Sprintf("change_%d", time.Now().Unix())

	// Store previous config for rollback
	prevConfig := cm.currentConfig

	// Apply the new configuration
	newConfig.Timestamp = time.Now()
	cm.currentConfig = newConfig

	// Record successful change
	change := ConfigChange{
		ID:          changeID,
		Timestamp:   time.Now(),
		ChangedBy:   changedBy,
		Section:     "all",
		Changes:     changes,
		PrevConfig:  prevConfig,
		Description: description,
		Success:     true,
	}

	cm.addToHistory(change)

	// Notify listeners
	cm.notifyListeners()

	log.Printf("Configuration updated successfully by %s: %s", changedBy, description)
	return nil
}

// UpdateSection updates a specific configuration section
func (cm *ConfigManager) UpdateSection(section string, sectionConfig interface{}, changedBy, description string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Create a copy of current config
	newConfig := cm.currentConfig
	configJSON, _ := json.Marshal(newConfig)
	var updatedConfig SystemConfig
	json.Unmarshal(configJSON, &updatedConfig)

	// Update the specific section
	switch section {
	case "address_pool":
		if poolConfig, ok := sectionConfig.(PoolConfig); ok {
			updatedConfig.AddressPool = poolConfig
		} else {
			return fmt.Errorf("invalid pool configuration type")
		}
	case "gap_monitor":
		if gapConfig, ok := sectionConfig.(GapConfig); ok {
			updatedConfig.GapMonitor = gapConfig
		} else {
			return fmt.Errorf("invalid gap monitor configuration type")
		}
	case "rate_limiter":
		if rateLimitConfig, ok := sectionConfig.(RateLimitConfig); ok {
			updatedConfig.RateLimiter = rateLimitConfig
		} else {
			return fmt.Errorf("invalid rate limiter configuration type")
		}
	case "websocket":
		if wsConfig, ok := sectionConfig.(WebSocketConfig); ok {
			updatedConfig.WebSocket = wsConfig
		} else {
			return fmt.Errorf("invalid websocket configuration type")
		}
	case "admin_dashboard":
		if adminConfig, ok := sectionConfig.(AdminConfig); ok {
			updatedConfig.AdminDashboard = adminConfig
		} else {
			return fmt.Errorf("invalid admin dashboard configuration type")
		}
	default:
		return fmt.Errorf("unknown configuration section: %s", section)
	}

	// Validate the updated configuration
	if err := cm.validateConfig(&updatedConfig); err != nil {
		cm.recordFailedChange(changedBy, section, description, err)
		return fmt.Errorf("validation failed: %w", err)
	}

	// Record the change
	changes := cm.detectChanges(cm.currentConfig, &updatedConfig)
	changeID := fmt.Sprintf("change_%d", time.Now().Unix())

	// Store previous config for rollback
	prevConfig := cm.currentConfig

	// Apply the new configuration
	updatedConfig.Timestamp = time.Now()
	cm.currentConfig = &updatedConfig

	// Record successful change
	change := ConfigChange{
		ID:          changeID,
		Timestamp:   time.Now(),
		ChangedBy:   changedBy,
		Section:     section,
		Changes:     changes,
		PrevConfig:  prevConfig,
		Description: description,
		Success:     true,
	}

	cm.addToHistory(change)

	// Notify listeners
	cm.notifyListeners()

	log.Printf("Configuration section '%s' updated successfully by %s: %s", section, changedBy, description)
	return nil
}

// RollbackToChange rolls back to a previous configuration
func (cm *ConfigManager) RollbackToChange(changeID, rolledBackBy string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Find the change in history
	var targetChange *ConfigChange
	for i := range cm.history {
		if cm.history[i].ID == changeID {
			targetChange = &cm.history[i]
			break
		}
	}

	if targetChange == nil {
		return fmt.Errorf("change ID %s not found in history", changeID)
	}

	if targetChange.PrevConfig == nil {
		return fmt.Errorf("previous configuration not available for change %s", changeID)
	}

	// Validate the rollback configuration
	if err := cm.validateConfig(targetChange.PrevConfig); err != nil {
		return fmt.Errorf("rollback validation failed: %w", err)
	}

	// Store current config before rollback
	prevConfig := cm.currentConfig

	// Apply the rollback
	rollbackConfig := targetChange.PrevConfig
	rollbackConfig.Timestamp = time.Now()
	cm.currentConfig = rollbackConfig

	// Record the rollback as a new change
	rollbackChange := ConfigChange{
		ID:          fmt.Sprintf("rollback_%d", time.Now().Unix()),
		Timestamp:   time.Now(),
		ChangedBy:   rolledBackBy,
		Section:     targetChange.Section,
		Changes:     []FieldChange{{Field: "rollback", OldValue: prevConfig, NewValue: rollbackConfig}},
		PrevConfig:  prevConfig,
		Description: fmt.Sprintf("Rollback to change %s", changeID),
		Success:     true,
	}

	cm.addToHistory(rollbackChange)

	// Notify listeners
	cm.notifyListeners()

	log.Printf("Configuration rolled back to change %s by %s", changeID, rolledBackBy)
	return nil
}

// GetHistory returns the configuration change history
func (cm *ConfigManager) GetHistory() []ConfigChange {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Return a copy of the history
	historyCopy := make([]ConfigChange, len(cm.history))
	copy(historyCopy, cm.history)

	return historyCopy
}

// RegisterChangeListener registers a callback for configuration changes
func (cm *ConfigManager) RegisterChangeListener(name string, callback func(*SystemConfig)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.changeListeners[name] = callback
	log.Printf("Configuration change listener '%s' registered", name)
}

// UnregisterChangeListener unregisters a configuration change callback
func (cm *ConfigManager) UnregisterChangeListener(name string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	delete(cm.changeListeners, name)
	log.Printf("Configuration change listener '%s' unregistered", name)
}

// validateConfig validates the entire configuration
func (cm *ConfigManager) validateConfig(config *SystemConfig) error {
	// Validate address pool config
	if config.AddressPool.MinPoolSize < 1 || config.AddressPool.MinPoolSize > 100 {
		return fmt.Errorf("min_pool_size must be between 1 and 100")
	}
	if config.AddressPool.MaxPoolSize < config.AddressPool.MinPoolSize || config.AddressPool.MaxPoolSize > 1000 {
		return fmt.Errorf("max_pool_size must be greater than min_pool_size and less than 1000")
	}
	if config.AddressPool.RefillThreshold < 1 || config.AddressPool.RefillThreshold > config.AddressPool.MinPoolSize {
		return fmt.Errorf("refill_threshold must be between 1 and min_pool_size")
	}

	// Validate gap monitor config
	if config.GapMonitor.MaxGapLimit < 10 || config.GapMonitor.MaxGapLimit > 100 {
		return fmt.Errorf("max_gap_limit must be between 10 and 100")
	}
	if config.GapMonitor.WarningThreshold < 0 || config.GapMonitor.WarningThreshold > 1 {
		return fmt.Errorf("warning_threshold must be between 0 and 1")
	}
	if config.GapMonitor.CriticalThreshold < 0 || config.GapMonitor.CriticalThreshold > 1 {
		return fmt.Errorf("critical_threshold must be between 0 and 1")
	}
	if config.GapMonitor.WarningThreshold >= config.GapMonitor.CriticalThreshold {
		return fmt.Errorf("warning_threshold must be less than critical_threshold")
	}

	// Validate rate limiter config
	if config.RateLimiter.GlobalMaxTokens < 50 || config.RateLimiter.GlobalMaxTokens > 1000 {
		return fmt.Errorf("global_max_tokens must be between 50 and 1000")
	}
	if config.RateLimiter.IPMaxTokens < 1 || config.RateLimiter.IPMaxTokens > 50 {
		return fmt.Errorf("ip_max_tokens must be between 1 and 50")
	}
	if config.RateLimiter.EmailMaxTokens < 1 || config.RateLimiter.EmailMaxTokens > 20 {
		return fmt.Errorf("email_max_tokens must be between 1 and 20")
	}

	// Validate WebSocket config
	if config.WebSocket.ReadBufferSize < 512 || config.WebSocket.ReadBufferSize > 8192 {
		return fmt.Errorf("read_buffer_size must be between 512 and 8192")
	}
	if config.WebSocket.WriteBufferSize < 512 || config.WebSocket.WriteBufferSize > 8192 {
		return fmt.Errorf("write_buffer_size must be between 512 and 8192")
	}

	return nil
}

// detectChanges compares two configurations and returns the differences
func (cm *ConfigManager) detectChanges(oldConfig, newConfig *SystemConfig) []FieldChange {
	changes := []FieldChange{}

	// Compare address pool config
	if oldConfig.AddressPool != newConfig.AddressPool {
		if oldConfig.AddressPool.MinPoolSize != newConfig.AddressPool.MinPoolSize {
			changes = append(changes, FieldChange{
				Field:    "address_pool.min_pool_size",
				OldValue: oldConfig.AddressPool.MinPoolSize,
				NewValue: newConfig.AddressPool.MinPoolSize,
			})
		}
		if oldConfig.AddressPool.MaxPoolSize != newConfig.AddressPool.MaxPoolSize {
			changes = append(changes, FieldChange{
				Field:    "address_pool.max_pool_size",
				OldValue: oldConfig.AddressPool.MaxPoolSize,
				NewValue: newConfig.AddressPool.MaxPoolSize,
			})
		}
		if oldConfig.AddressPool.RefillThreshold != newConfig.AddressPool.RefillThreshold {
			changes = append(changes, FieldChange{
				Field:    "address_pool.refill_threshold",
				OldValue: oldConfig.AddressPool.RefillThreshold,
				NewValue: newConfig.AddressPool.RefillThreshold,
			})
		}
	}

	// Compare gap monitor config
	if oldConfig.GapMonitor != newConfig.GapMonitor {
		if oldConfig.GapMonitor.MaxGapLimit != newConfig.GapMonitor.MaxGapLimit {
			changes = append(changes, FieldChange{
				Field:    "gap_monitor.max_gap_limit",
				OldValue: oldConfig.GapMonitor.MaxGapLimit,
				NewValue: newConfig.GapMonitor.MaxGapLimit,
			})
		}
		if oldConfig.GapMonitor.WarningThreshold != newConfig.GapMonitor.WarningThreshold {
			changes = append(changes, FieldChange{
				Field:    "gap_monitor.warning_threshold",
				OldValue: oldConfig.GapMonitor.WarningThreshold,
				NewValue: newConfig.GapMonitor.WarningThreshold,
			})
		}
		if oldConfig.GapMonitor.CriticalThreshold != newConfig.GapMonitor.CriticalThreshold {
			changes = append(changes, FieldChange{
				Field:    "gap_monitor.critical_threshold",
				OldValue: oldConfig.GapMonitor.CriticalThreshold,
				NewValue: newConfig.GapMonitor.CriticalThreshold,
			})
		}
	}

	// Compare rate limiter config (simplified)
	if oldConfig.RateLimiter != newConfig.RateLimiter {
		if oldConfig.RateLimiter.GlobalMaxTokens != newConfig.RateLimiter.GlobalMaxTokens {
			changes = append(changes, FieldChange{
				Field:    "rate_limiter.global_max_tokens",
				OldValue: oldConfig.RateLimiter.GlobalMaxTokens,
				NewValue: newConfig.RateLimiter.GlobalMaxTokens,
			})
		}
	}

	return changes
}

// addToHistory adds a change to the history, maintaining size limits
func (cm *ConfigManager) addToHistory(change ConfigChange) {
	cm.history = append(cm.history, change)

	// Maintain history size limit
	if len(cm.history) > cm.maxHistorySize {
		cm.history = cm.history[1:] // Remove oldest entry
	}
}

// recordFailedChange records a failed configuration change
func (cm *ConfigManager) recordFailedChange(changedBy, section, description string, err error) {
	change := ConfigChange{
		ID:          fmt.Sprintf("failed_%d", time.Now().Unix()),
		Timestamp:   time.Now(),
		ChangedBy:   changedBy,
		Section:     section,
		Changes:     []FieldChange{},
		Description: description,
		Success:     false,
		ErrorMsg:    err.Error(),
	}

	cm.addToHistory(change)
	log.Printf("Failed configuration change by %s: %s - %v", changedBy, description, err)
}

// notifyListeners notifies all registered change listeners
func (cm *ConfigManager) notifyListeners() {
	for name, callback := range cm.changeListeners {
		go func(listenerName string, cb func(*SystemConfig)) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Configuration listener '%s' panicked: %v", listenerName, r)
				}
			}()
			cb(cm.currentConfig)
		}(name, callback)
	}
}

// GetValidationRules returns validation rules for the frontend
func (cm *ConfigManager) GetValidationRules() map[string]interface{} {
	return map[string]interface{}{
		"address_pool": map[string]interface{}{
			"min_pool_size":     map[string]int{"min": 1, "max": 100},
			"max_pool_size":     map[string]int{"min": 5, "max": 1000},
			"refill_threshold":  map[string]int{"min": 1, "max": 100},
			"refill_batch_size": map[string]int{"min": 1, "max": 50},
			"cleanup_interval":  map[string]int{"min": 5, "max": 1440},
		},
		"gap_monitor": map[string]interface{}{
			"max_gap_limit":              map[string]int{"min": 10, "max": 100},
			"warning_threshold":          map[string]float64{"min": 0.0, "max": 1.0},
			"critical_threshold":         map[string]float64{"min": 0.0, "max": 1.0},
			"consecutive_fail_threshold": map[string]int{"min": 1, "max": 20},
			"error_history_size":         map[string]int{"min": 10, "max": 1000},
		},
		"rate_limiter": map[string]interface{}{
			"global_max_tokens":  map[string]int{"min": 50, "max": 1000},
			"global_refill_rate": map[string]int{"min": 1, "max": 100},
			"ip_max_tokens":      map[string]int{"min": 1, "max": 50},
			"ip_refill_rate":     map[string]int{"min": 1, "max": 20},
			"email_max_tokens":   map[string]int{"min": 1, "max": 20},
			"email_refill_rate":  map[string]int{"min": 1, "max": 10},
			"cleanup_interval":   map[string]int{"min": 5, "max": 1440},
		},
		"websocket": map[string]interface{}{
			"read_buffer_size":  map[string]int{"min": 512, "max": 8192},
			"write_buffer_size": map[string]int{"min": 512, "max": 8192},
			"ping_interval":     map[string]int{"min": 10, "max": 300},
			"pong_timeout":      map[string]int{"min": 5, "max": 120},
			"max_connections":   map[string]int{"min": 10, "max": 10000},
		},
		"admin_dashboard": map[string]interface{}{
			"session_timeout":   map[string]int{"min": 1, "max": 168},
			"refresh_interval":  map[string]int{"min": 5, "max": 300},
			"max_log_entries":   map[string]int{"min": 100, "max": 10000},
			"metrics_retention": map[string]int{"min": 1, "max": 8760},
		},
	}
}
