package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the application
type Config struct {
	// Server settings
	Port              string
	Environment       string
	MaxConnections    int
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	
	// Resource limits
	MaxGoroutines     int
	MaxMemoryMB       int
	MaxWebSockets     int
	MaxSSEConnections int
	
	// Polling intervals
	DefaultPollInterval time.Duration
	FastPollInterval    time.Duration
	
	// Connection pooling
	MaxIdleConns        int
	MaxOpenConns        int
	ConnMaxLifetime     time.Duration
	
	// Rate limiting
	RequestsPerMinute   int
	BurstSize          int
	
	// Cleanup intervals
	CleanupInterval     time.Duration
	SessionTimeout      time.Duration
}

// Load returns configuration from environment variables with defaults optimized for Render.com free tier
func Load() *Config {
	return &Config{
		// Server settings
		Port:            getEnv("PORT", "8080"),
		Environment:     getEnv("ENVIRONMENT", "production"),
		MaxConnections:  getEnvAsInt("MAX_CONNECTIONS", 100),
		ReadTimeout:     getEnvAsDuration("READ_TIMEOUT", "30s"),
		WriteTimeout:    getEnvAsDuration("WRITE_TIMEOUT", "30s"),
		IdleTimeout:     getEnvAsDuration("IDLE_TIMEOUT", "120s"),
		ShutdownTimeout: getEnvAsDuration("SHUTDOWN_TIMEOUT", "10s"),
		
		// Resource limits - optimized for 512MB RAM on Render free tier
		MaxGoroutines:     getEnvAsInt("MAX_GOROUTINES", 50),
		MaxMemoryMB:       getEnvAsInt("MAX_MEMORY_MB", 400),
		MaxWebSockets:     getEnvAsInt("MAX_WEBSOCKETS", 50),
		MaxSSEConnections: getEnvAsInt("MAX_SSE_CONNECTIONS", 50),
		
		// Polling intervals
		DefaultPollInterval: getEnvAsDuration("DEFAULT_POLL_INTERVAL", "60s"),
		FastPollInterval:    getEnvAsDuration("FAST_POLL_INTERVAL", "15s"),
		
		// Connection pooling - reduced for lower resource usage
		MaxIdleConns:    getEnvAsInt("MAX_IDLE_CONNS", 5),
		MaxOpenConns:    getEnvAsInt("MAX_OPEN_CONNS", 10),
		ConnMaxLifetime: getEnvAsDuration("CONN_MAX_LIFETIME", "5m"),
		
		// Rate limiting
		RequestsPerMinute: getEnvAsInt("REQUESTS_PER_MINUTE", 60),
		BurstSize:        getEnvAsInt("BURST_SIZE", 10),
		
		// Cleanup intervals
		CleanupInterval: getEnvAsDuration("CLEANUP_INTERVAL", "5m"),
		SessionTimeout:  getEnvAsDuration("SESSION_TIMEOUT", "35m"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue string) time.Duration {
	valueStr := getEnv(key, defaultValue)
	if duration, err := time.ParseDuration(valueStr); err == nil {
		return duration
	}
	// Return default if parsing fails
	duration, _ := time.ParseDuration(defaultValue)
	return duration
}