package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/ngenohkevin/paybutton/internals/config"
	"github.com/ngenohkevin/paybutton/internals/database"
	"github.com/ngenohkevin/paybutton/internals/monitoring"
	"github.com/ngenohkevin/paybutton/internals/payment_processing"
	"github.com/ngenohkevin/paybutton/internals/server"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Set up memory limits for Render.com free tier
	debug.SetMemoryLimit(int64(cfg.MaxMemoryMB) * 1024 * 1024)

	// Optimize Go runtime for low memory
	runtime.GOMAXPROCS(2)  // Limit CPU cores for free tier
	debug.SetGCPercent(20) // More aggressive GC for lower memory usage

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Initialize resource monitor
	monitor := monitoring.InitResourceMonitor(
		cfg.MaxGoroutines,
		cfg.MaxMemoryMB,
		cfg.CleanupInterval,
	)
	defer monitor.Shutdown()

	logger.Info("Starting PayButton service with resource limits",
		slog.Int("max_goroutines", cfg.MaxGoroutines),
		slog.Int("max_memory_mb", cfg.MaxMemoryMB),
		slog.String("environment", cfg.Environment),
	)

	//Init the DB with connection pooling (Supabase - for other app data)
	err := database.InitDBWithConfig(cfg.MaxIdleConns, cfg.MaxOpenConns, cfg.ConnMaxLifetime)
	if err != nil {
		logger.Error("Error initializing database:", slog.String("error", err.Error()))
		os.Exit(1)
	}

	//closing the db when the app exits
	defer database.CloseDB()

	// Initialize PayButton pool database (separate Postgres for address pool persistence)
	if database.IsEnabled() {
		if err := database.Initialize(); err != nil {
			logger.Error("⚠️ Failed to initialize pool database - running in memory-only mode",
				slog.String("error", err.Error()))
		} else {
			defer database.Close()
			logger.Info("✅ PayButton pool database initialized successfully")
		}
	} else {
		logger.Info("⚠️ Pool persistence disabled - running in memory-only mode")
	}

	// Initialize address pools for all sites (will load from database if enabled)
	payment_processing.InitializeAddressPools()

	// Start background cleanup job for expired payments
	go payment_processing.StartPaymentCleanupJob()

	// Start comprehensive address health service (runs once per day)
	if database.Queries != nil {
		healthService := payment_processing.NewAddressHealthService(database.Queries, 24*time.Hour)
		healthService.Start()
		defer healthService.Stop()
		logger.Info("✅ Address Health Service started (runs daily)")
	} else {
		logger.Info("⚠️ Address Health Service disabled - no database connection")
	}

	srv, err := server.NewServerWithConfig(logger, cfg)
	if err != nil {
		logger.Error("Error creating server:", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Shutdown signal received, gracefully stopping...")
		cancel()
		srv.Shutdown(context.Background())
	}()

	//start the server
	go func() {
		if err := srv.StartWithContext(ctx); err != nil && err != http.ErrServerClosed {
			logger.Error("Error starting server:", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	logger.Info("server is starting on port " + cfg.Port)

	// Wait for shutdown signal
	<-ctx.Done()
	logger.Info("Shutting down gracefully...")
}
