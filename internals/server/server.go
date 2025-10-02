package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/ngenohkevin/paybutton/internals/analytics"
	"github.com/ngenohkevin/paybutton/internals/config"
	"github.com/ngenohkevin/paybutton/internals/monitoring"
	"github.com/ngenohkevin/paybutton/internals/payment_processing"
	"github.com/ngenohkevin/paybutton/utils"
)

type Server struct {
	logger     *slog.Logger
	httpServer *http.Server
	config     *config.Config
}

func NewServer(logger *slog.Logger) (*Server, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	return &Server{
		logger: logger,
		config: config.Load(),
	}, nil
}

func NewServerWithConfig(logger *slog.Logger, cfg *config.Config) (*Server, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if cfg == nil {
		cfg = config.Load()
	}
	return &Server{
		logger: logger,
		config: cfg,
	}, nil
}

func (s *Server) Start() error {
	s.logger.Info("Starting server")

	botToken, err := utils.LoadConfig()
	if err != nil {
		s.logger.Error("Error loading config:", "error", err)
	}

	// Initialize API keys for payment processing
	if err := payment_processing.InitializeAPIKeys(); err != nil {
		s.logger.Error("Error initializing API keys:", "error", err)
	}

	// Initialize alert management system
	monitoring.InitializeAlertManager()
	s.logger.Info("Alert management system initialized")

	// Initialize analytics system
	analytics.Initialize()
	s.logger.Info("Analytics system initialized")

	bot, err := tgbotapi.NewBotAPI(botToken.BotApiKey)
	if err != nil {
		s.logger.Error("Error initializing bot:", "error", err)
		log.Printf("Bot initialized in development mode for sending only")
		// Continue without bot for development/testing
		bot = nil
	}

	// Helper function to safely send bot messages
	safeBotSend := func(msg tgbotapi.Chattable) (tgbotapi.Message, error) {
		if bot == nil {
			log.Printf("Bot message skipped (dev mode): %v", msg)
			return tgbotapi.Message{}, fmt.Errorf("bot not available")
		}
		return bot.Send(msg)
	}

	// Initialize bot delivery commands for manual and automatic delivery
	if bot != nil {
		payment_processing.SetupBotDeliveryCommands(bot)
	}

	//define the router
	r := gin.Default()
	r.Use(cors.Default())

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "version": "1.0.0"})
	})

	// Add no-cache middleware for development (prevents caching issues with Air)
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/static/") || strings.HasPrefix(c.Request.URL.Path, "/admin/") {
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
		}
		c.Next()
	})

	// Serve static files for admin UI
	r.Static("/static", "./static")

	r.POST("/cards", handlePayment(bot))
	r.POST("/usdt", handleUsdtPayment(bot))
	r.POST("/payment", handlePayment(bot))
	r.POST("/payment-fast", handleFastPayment(bot))                                                       // New endpoint for 15s polling
	r.GET("/ws/balance/:address", payment_processing.HandleWebSocket)                                     // WebSocket endpoint for real-time updates
	r.GET("/events/balance/:address", payment_processing.HandleSSE)                                       // SSE endpoint for real-time updates (lightweight)
	r.POST("/webhook/btc", func(c *gin.Context) { payment_processing.HandleBlockonomicsWebhook(c, bot) }) // Blockonomics webhook
	r.GET("/balance/:address", payment_processing.GetBalance)

	// Analytics WebSocket endpoint for site visitor tracking
	r.GET("/ws/analytics/:siteName", analytics.HandleWebSocket)

	// Enhanced Analytics v2 WebSocket endpoint with improved features
	r.GET("/ws/analytics/v2/:siteName", analytics.HandleEnhancedWebSocket)

	// Admin Analytics WebSocket endpoint for real-time dashboard updates
	r.GET("/ws/admin/analytics", analytics.HandleAdminWebSocket)

	// Beacon API fallback endpoint for analytics
	r.POST("/analytics/beacon", analytics.HandleBeacon)

	// Analytics SDK endpoints
	r.GET("/analytics.js", serveAnalyticsSDK)
	r.GET("/analytics-v2.js", serveAnalyticsSDKv2)

	// Initialize admin authentication and UI
	adminAuth := NewAdminAuth()

	// Register admin endpoints for monitoring and management
	RegisterAdminEndpoints(r, adminAuth)

	// Set up session tracking callback to connect payment processing with admin dashboard
	payment_processing.SessionTracker = AddSession
	payment_processing.SessionStatusUpdater = UpdateSessionStatusByAddress
	payment_processing.SessionWebSocketUpdater = UpdateSessionWebSocketByAddress

	// Add the webhook endpoint for Telegram
	webhookPath := "/bot" + botToken.BotApiKey + "/webhook"
	r.POST(webhookPath, func(c *gin.Context) {
		bytes, err := c.GetRawData()
		if err != nil {
			log.Printf("Error getting raw data: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
			return
		}

		var update tgbotapi.Update
		err = json.Unmarshal(bytes, &update)
		if err != nil {
			log.Printf("Error decoding update: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
			return
		}

		// Process the update
		if update.Message != nil {
			message := update.Message.Text
			// We don't need this here, using update.Message.Chat.ID directly
			// chatID := payment_processing.GetChatID()

			// Handle manual product delivery for USDT and BTC
			if strings.HasPrefix(message, "/deliver") || strings.HasPrefix(message, "!deliver") {
				// Extract the command part
				commandParts := strings.SplitN(message, " ", 2)
				if len(commandParts) < 2 {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						"❌ Invalid format. Use:\n/deliver <email> <n> <product>\nor\n/deliver **Email:** `email` **Name:** `name` **Product:** `product`")
					safeBotSend(msg)
					c.JSON(http.StatusOK, gin.H{"ok": true})
					return
				}

				// Get the command content (everything after the first space)
				commandContent := commandParts[1]

				// Parse the delivery command
				email, name, product, err := payment_processing.ParseDeliveryCommand(commandContent)
				if err != nil {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						fmt.Sprintf("❌ Failed to parse command: %v", err))
					safeBotSend(msg)
					c.JSON(http.StatusOK, gin.H{"ok": true})
					return
				}

				// Use default name if empty
				if name == "" {
					name = "Customer"
				}

				// Send the product email with progress indication
				statusMsg := tgbotapi.NewMessage(update.Message.Chat.ID,
					fmt.Sprintf("⏳ Processing delivery for %s...", email))
				statusMsgSent, _ := safeBotSend(statusMsg)

				err = payment_processing.HandleManualProductDelivery(email, name, product, bot, update.Message.Chat.ID)
				if err != nil {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						fmt.Sprintf("❌ Delivery failed: %v", err))
					safeBotSend(msg)
				} else {
					// Edit the previous message to show success
					editMsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, statusMsgSent.MessageID,
						fmt.Sprintf("✅ Product delivered successfully to %s!", email))
					safeBotSend(editMsg)
				}
			} else if strings.HasPrefix(message, "/balance") || strings.HasPrefix(message, "!balance") {
				// Handle balance email delivery for USDT
				parts := strings.SplitN(message, " ", 4) // Command, email, name, amount
				if len(parts) < 4 {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						"❌ Invalid format. Use: /balance <email> <n> <amount>")
					safeBotSend(msg)
					c.JSON(http.StatusOK, gin.H{"ok": true})
					return
				}

				email := strings.TrimSpace(parts[1])
				name := strings.TrimSpace(parts[2])
				amount := strings.TrimSpace(parts[3])

				// Validate email format
				if !strings.Contains(email, "@") {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "❌ Invalid email format")
					safeBotSend(msg)
					c.JSON(http.StatusOK, gin.H{"ok": true})
					return
				}

				// Use default name if empty
				if name == "" {
					name = "Customer"
				}

				// Validate amount
				amountFloat, err := utils.ParseFloat(amount)
				if err != nil {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "❌ Invalid amount format. Use decimal number (e.g., 10.50)")
					safeBotSend(msg)
					c.JSON(http.StatusOK, gin.H{"ok": true})
					return
				}

				// Format amount with 2 decimal places for consistency
				amount = fmt.Sprintf("%.2f", amountFloat)

				// Send the balance email with progress indication
				statusMsg := tgbotapi.NewMessage(update.Message.Chat.ID,
					fmt.Sprintf("⏳ Sending balance confirmation email to %s...", email))
				statusMsgSent, _ := safeBotSend(statusMsg)

				err = payment_processing.HandleManualBalanceEmailDelivery(email, name, amount, bot, update.Message.Chat.ID)
				if err != nil {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						fmt.Sprintf("❌ Balance email failed: %v", err))
					safeBotSend(msg)
				} else {
					// Edit the previous message to show success
					editMsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, statusMsgSent.MessageID,
						fmt.Sprintf("✅ Balance confirmation email sent successfully to %s!", email))
					safeBotSend(editMsg)
				}
			} else if strings.HasPrefix(message, "/help") || strings.HasPrefix(message, "!help") {
				// Send help information
				helpMsg := "Manual Delivery Commands\n\n" +
					"1. Product Delivery\n" +
					"   Two formats available:\n\n" +
					"   /deliver <email> <n> <product>\n" +
					"   Example: /deliver user@example.com John \"Premium Log\"\n\n" +
					"   OR\n\n" +
					"   /deliver <notification>\n" +
					"   Example: /deliver **Email:** user@example.com **Name:** John **Product:** Premium Log\n\n" +
					"2. Balance Added Email\n" +
					"   /balance <email> <n> <amount>\n\n" +
					"   Example: /balance user@example.com John 49.99\n\n" +
					"These commands let you manually process USDT or other cryptocurrency transactions."

				msg := tgbotapi.NewMessage(update.Message.Chat.ID, helpMsg)
				safeBotSend(msg)
			}
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// Configure HTTP server with timeouts
	s.httpServer = &http.Server{
		Addr:         ":" + s.config.Port,
		Handler:      r,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	err = s.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Fatalf("Failed to run server: %v", err)
	}

	return nil
}

func (s *Server) StartWithContext(ctx context.Context) error {
	s.logger.Info("Starting server with context")

	botToken, err := utils.LoadConfig()
	if err != nil {
		s.logger.Error("Error loading config:", "error", err)
	}

	// Initialize API keys for payment processing
	if err := payment_processing.InitializeAPIKeys(); err != nil {
		s.logger.Error("Error initializing API keys:", "error", err)
	}

	// Initialize alert management system
	monitoring.InitializeAlertManager()
	s.logger.Info("Alert management system initialized")

	// Initialize analytics system
	analytics.Initialize()
	s.logger.Info("Analytics system initialized")

	bot, err := tgbotapi.NewBotAPI(botToken.BotApiKey)
	if err != nil {
		s.logger.Error("Error initializing bot:", "error", err)
		log.Printf("Bot initialized in development mode for sending only")
		bot = nil
	}

	// Helper function to safely send bot messages
	safeBotSend := func(msg tgbotapi.Chattable) (tgbotapi.Message, error) {
		if bot == nil {
			log.Printf("Bot message skipped (dev mode): %v", msg)
			return tgbotapi.Message{}, fmt.Errorf("bot not available")
		}
		return bot.Send(msg)
	}

	// Initialize bot delivery commands for manual and automatic delivery
	if bot != nil {
		payment_processing.SetupBotDeliveryCommands(bot)
	}

	//define the router
	r := gin.Default()
	r.Use(cors.Default())

	// Add rate limiting middleware
	r.Use(s.rateLimitMiddleware())

	// Add WebSocket connection limiting
	r.Use(s.websocketLimitMiddleware())

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "version": "1.0.0"})
	})

	// Add no-cache middleware for development (prevents caching issues with Air)
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/static/") || strings.HasPrefix(c.Request.URL.Path, "/admin/") {
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
		}
		c.Next()
	})

	// Serve static files for admin UI
	r.Static("/static", "./static")

	r.POST("/cards", handlePayment(bot))
	r.POST("/usdt", handleUsdtPayment(bot))
	r.POST("/payment", handlePayment(bot))
	r.POST("/payment-fast", handleFastPayment(bot))
	r.GET("/ws/balance/:address", s.handleWebSocketWithLimit)
	r.GET("/events/balance/:address", s.handleSSEWithLimit)
	r.POST("/webhook/btc", func(c *gin.Context) { payment_processing.HandleBlockonomicsWebhook(c, bot) })
	r.GET("/balance/:address", payment_processing.GetBalance)

	// Analytics WebSocket endpoint for site visitor tracking
	r.GET("/ws/analytics/:siteName", analytics.HandleWebSocket)

	// Enhanced Analytics v2 WebSocket endpoint with improved features
	r.GET("/ws/analytics/v2/:siteName", analytics.HandleEnhancedWebSocket)

	// Admin Analytics WebSocket endpoint for real-time dashboard updates
	r.GET("/ws/admin/analytics", analytics.HandleAdminWebSocket)

	// Beacon API fallback endpoint for analytics
	r.POST("/analytics/beacon", analytics.HandleBeacon)

	// Analytics SDK endpoints
	r.GET("/analytics.js", serveAnalyticsSDK)
	r.GET("/analytics-v2.js", serveAnalyticsSDKv2)

	// Initialize admin authentication and UI
	adminAuth := NewAdminAuth()

	// Register admin endpoints for monitoring and management
	RegisterAdminEndpoints(r, adminAuth)

	// Set up session tracking callback to connect payment processing with admin dashboard
	payment_processing.SessionTracker = AddSession
	payment_processing.SessionStatusUpdater = UpdateSessionStatusByAddress
	payment_processing.SessionWebSocketUpdater = UpdateSessionWebSocketByAddress

	// Add the webhook endpoint for Telegram
	webhookPath := "/bot" + botToken.BotApiKey + "/webhook"
	r.POST(webhookPath, func(c *gin.Context) {
		bytes, err := c.GetRawData()
		if err != nil {
			log.Printf("Error getting raw data: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
			return
		}

		var update tgbotapi.Update
		err = json.Unmarshal(bytes, &update)
		if err != nil {
			log.Printf("Error decoding update: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
			return
		}

		// Process the update
		if update.Message != nil {
			message := update.Message.Text

			// Handle manual product delivery for USDT and BTC
			if strings.HasPrefix(message, "/deliver") || strings.HasPrefix(message, "!deliver") {
				// Extract the command part
				commandParts := strings.SplitN(message, " ", 2)
				if len(commandParts) < 2 {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						"❌ Invalid format. Use:\n/deliver <email> <n> <product>\nor\n/deliver **Email:** `email` **Name:** `name` **Product:** `product`")
					safeBotSend(msg)
					c.JSON(http.StatusOK, gin.H{"ok": true})
					return
				}

				// Get the command content (everything after the first space)
				commandContent := commandParts[1]

				// Parse the delivery command
				email, name, product, err := payment_processing.ParseDeliveryCommand(commandContent)
				if err != nil {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						fmt.Sprintf("❌ Failed to parse command: %v", err))
					safeBotSend(msg)
					c.JSON(http.StatusOK, gin.H{"ok": true})
					return
				}

				// Use default name if empty
				if name == "" {
					name = "Customer"
				}

				// Send the product email with progress indication
				statusMsg := tgbotapi.NewMessage(update.Message.Chat.ID,
					fmt.Sprintf("⏳ Processing delivery for %s...", email))
				statusMsgSent, _ := safeBotSend(statusMsg)

				err = payment_processing.HandleManualProductDelivery(email, name, product, bot, update.Message.Chat.ID)
				if err != nil {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						fmt.Sprintf("❌ Delivery failed: %v", err))
					safeBotSend(msg)
				} else {
					// Edit the previous message to show success
					editMsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, statusMsgSent.MessageID,
						fmt.Sprintf("✅ Product delivered successfully to %s!", email))
					safeBotSend(editMsg)
				}
			} else if strings.HasPrefix(message, "/balance") || strings.HasPrefix(message, "!balance") {
				// Handle balance email delivery for USDT
				parts := strings.SplitN(message, " ", 4) // Command, email, name, amount
				if len(parts) < 4 {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						"❌ Invalid format. Use: /balance <email> <n> <amount>")
					safeBotSend(msg)
					c.JSON(http.StatusOK, gin.H{"ok": true})
					return
				}

				email := strings.TrimSpace(parts[1])
				name := strings.TrimSpace(parts[2])
				amount := strings.TrimSpace(parts[3])

				// Validate email format
				if !strings.Contains(email, "@") {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "❌ Invalid email format")
					safeBotSend(msg)
					c.JSON(http.StatusOK, gin.H{"ok": true})
					return
				}

				// Use default name if empty
				if name == "" {
					name = "Customer"
				}

				// Validate amount
				amountFloat, err := utils.ParseFloat(amount)
				if err != nil {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "❌ Invalid amount format. Use decimal number (e.g., 10.50)")
					safeBotSend(msg)
					c.JSON(http.StatusOK, gin.H{"ok": true})
					return
				}

				// Format amount with 2 decimal places for consistency
				amount = fmt.Sprintf("%.2f", amountFloat)

				// Send the balance email with progress indication
				statusMsg := tgbotapi.NewMessage(update.Message.Chat.ID,
					fmt.Sprintf("⏳ Sending balance confirmation email to %s...", email))
				statusMsgSent, _ := safeBotSend(statusMsg)

				err = payment_processing.HandleManualBalanceEmailDelivery(email, name, amount, bot, update.Message.Chat.ID)
				if err != nil {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						fmt.Sprintf("❌ Balance email failed: %v", err))
					safeBotSend(msg)
				} else {
					// Edit the previous message to show success
					editMsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, statusMsgSent.MessageID,
						fmt.Sprintf("✅ Balance confirmation email sent successfully to %s!", email))
					safeBotSend(editMsg)
				}
			} else if strings.HasPrefix(message, "/help") || strings.HasPrefix(message, "!help") {
				// Send help information
				helpMsg := "Manual Delivery Commands\n\n" +
					"1. Product Delivery\n" +
					"   Two formats available:\n\n" +
					"   /deliver <email> <n> <product>\n" +
					"   Example: /deliver user@example.com John \"Premium Log\"\n\n" +
					"   OR\n\n" +
					"   /deliver <notification>\n" +
					"   Example: /deliver **Email:** user@example.com **Name:** John **Product:** Premium Log\n\n" +
					"2. Balance Added Email\n" +
					"   /balance <email> <n> <amount>\n\n" +
					"   Example: /balance user@example.com John 49.99\n\n" +
					"These commands let you manually process USDT or other cryptocurrency transactions."

				msg := tgbotapi.NewMessage(update.Message.Chat.ID, helpMsg)
				safeBotSend(msg)
			}
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// Configure HTTP server with timeouts
	s.httpServer = &http.Server{
		Addr:         ":" + s.config.Port,
		Handler:      r,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	s.logger.Info("Starting HTTP server", slog.String("address", s.httpServer.Addr))

	// Start server in goroutine
	go func() {
		s.logger.Info("HTTP server listening", slog.String("address", s.httpServer.Addr))
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Server error:", slog.String("error", err.Error()))
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	return s.Shutdown(context.Background())
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down server...")

	// Shutdown analytics system first to close connections gracefully
	analytics.Shutdown()
	s.logger.Info("Analytics system shutdown completed")

	if s.httpServer == nil {
		return nil
	}

	// Give outstanding requests time to complete
	shutdownCtx, cancel := context.WithTimeout(ctx, s.config.ShutdownTimeout)
	defer cancel()

	return s.httpServer.Shutdown(shutdownCtx)
}

func handlePayment(bot *tgbotapi.BotAPI) gin.HandlerFunc {
	return func(c *gin.Context) {
		payment_processing.ProcessPaymentRequest(c, bot, true, false)
	}
}

func handleUsdtPayment(bot *tgbotapi.BotAPI) gin.HandlerFunc {
	return func(c *gin.Context) {
		payment_processing.ProcessPaymentRequest(c, bot, false, true)
	}
}

func handleFastPayment(bot *tgbotapi.BotAPI) gin.HandlerFunc {
	return func(c *gin.Context) {
		payment_processing.ProcessFastPaymentRequest(c, bot, true, false)
	}
}

// serveAnalyticsSDK serves the analytics JavaScript SDK
func serveAnalyticsSDK(c *gin.Context) {
	// Set appropriate headers for JavaScript
	c.Header("Content-Type", "application/javascript; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate") // No caching during development
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Header("Access-Control-Allow-Origin", "*") // Allow cross-origin requests
	c.Header("Access-Control-Allow-Methods", "GET")
	c.Header("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept")

	// Serve the analytics.js file
	c.File("./static/js/analytics.js")
}

// serveAnalyticsSDKv2 serves the enhanced analytics v2 JavaScript SDK
func serveAnalyticsSDKv2(c *gin.Context) {
	// Set appropriate headers for JavaScript
	c.Header("Content-Type", "application/javascript; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=3600") // Cache for 1 hour in production
	c.Header("Access-Control-Allow-Origin", "*")      // Allow cross-origin requests
	c.Header("Access-Control-Allow-Methods", "GET")
	c.Header("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept")
	c.Header("X-Content-Type-Options", "nosniff")

	// Add version header for debugging
	c.Header("X-Analytics-Version", "2.0.0")

	// Serve the analytics-v2.js file
	c.File("./static/js/analytics-v2.js")
}
