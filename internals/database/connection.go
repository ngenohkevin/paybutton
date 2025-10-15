package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ngenohkevin/paybutton/internals/db"
)

var (
	Pool    *pgxpool.Pool
	Queries *db.Queries
)

type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// LoadConfig loads database configuration from environment variables
func LoadConfig() Config {
	return Config{
		Host:     getEnv("PAYBUTTON_DB_HOST", "localhost"),
		Port:     getEnv("PAYBUTTON_DB_PORT", "5432"),
		User:     getEnv("PAYBUTTON_DB_USER", "postgres"),
		Password: getEnv("PAYBUTTON_DB_PASSWORD", ""),
		DBName:   getEnv("PAYBUTTON_DB_NAME", "paybutton_pool"),
		SSLMode:  getEnv("PAYBUTTON_DB_SSLMODE", "disable"),
	}
}

// ConnectionString builds PostgreSQL connection string
func (c Config) ConnectionString() string {
	return fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.DBName, c.SSLMode)
}

// Initialize sets up database connection pool
func Initialize() error {
	// First try to use DATABASE_URL if available (for external/local access)
	var connString string
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		connString = databaseURL
		log.Printf("Using DATABASE_URL for connection")
	} else {
		// Fall back to individual config variables (for internal Dokploy access)
		config := LoadConfig()
		connString = config.ConnectionString()
		log.Printf("Using individual config variables for connection")
	}

	// Configure pool
	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return fmt.Errorf("failed to parse database config: %w", err)
	}

	// Connection pool settings
	poolConfig.MaxConns = 25
	poolConfig.MinConns = 5
	poolConfig.MaxConnLifetime = 5 * time.Minute
	poolConfig.MaxConnIdleTime = 2 * time.Minute
	poolConfig.HealthCheckPeriod = 1 * time.Minute

	// Create connection pool
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	Pool, err = pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection
	if err := Pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Initialize sqlc queries
	Queries = db.New(Pool)

	log.Printf("✅ Database connection established successfully")
	log.Printf("   Connection: %s", connString)
	log.Printf("   Pool: %d-%d connections", poolConfig.MinConns, poolConfig.MaxConns)

	return nil
}

// Close closes the database connection pool
func Close() {
	if Pool != nil {
		Pool.Close()
		log.Println("Database connection pool closed")
	}
}

// Health checks database health
func Health(ctx context.Context) error {
	if Pool == nil {
		return fmt.Errorf("database pool not initialized")
	}

	if err := Pool.Ping(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	stats := Pool.Stat()
	log.Printf("Database health: %d/%d connections in use",
		stats.AcquiredConns(), stats.TotalConns())

	return nil
}

// IsEnabled checks if database persistence is enabled
func IsEnabled() bool {
	enabled := getEnv("ENABLE_POOL_PERSISTENCE", "false")
	return enabled == "true" || enabled == "1"
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
