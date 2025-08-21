package server

import (
	"encoding/json"
	"fmt"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/ngenohkevin/paybutton/internals/payment_processing"
	"github.com/ngenohkevin/paybutton/utils"
	"log"
	"log/slog"
	"net/http"
	"strings"
)

type Server struct {
	logger     *slog.Logger
	httpServer *http.Server
}

func NewServer(logger *slog.Logger) (*Server, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	return &Server{
		logger: logger,
	}, nil
}

func (s *Server) Start() error {
	s.logger.Info("Starting server")

	botToken, err := utils.LoadConfig()
	if err != nil {
		s.logger.Error("Error loading config:", err)
	}

	// Initialize API keys for payment processing
	if err := payment_processing.InitializeAPIKeys(); err != nil {
		s.logger.Error("Error initializing API keys:", err)
	}

	bot, err := tgbotapi.NewBotAPI(botToken.BotApiKey)
	if err != nil {
		s.logger.Error("Error initializing bot:", err)
		return fmt.Errorf("failed to initialize Telegram bot: %w", err)
	}

	// Initialize bot delivery commands for manual and automatic delivery
	payment_processing.SetupBotDeliveryCommands(bot)

	//define the router
	r := gin.Default()
	r.Use(cors.Default())

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "version": "1.0.0"})
	})

	r.POST("/cards", handlePayment(bot))
	r.POST("/usdt", handleUsdtPayment(bot))
	r.POST("/payment", handlePayment(bot))
	r.POST("/payment-fast", handleFastPayment(bot))                                                       // New endpoint for 15s polling
	r.GET("/ws/balance/:address", payment_processing.HandleWebSocket)                                     // WebSocket endpoint for real-time updates
	r.GET("/events/balance/:address", payment_processing.HandleSSE)                                       // SSE endpoint for real-time updates (lightweight)
	r.POST("/webhook/btc", func(c *gin.Context) { payment_processing.HandleBlockonomicsWebhook(c, bot) }) // Blockonomics webhook
	r.GET("/balance/:address", payment_processing.GetBalance)
	
	// Register admin endpoints for monitoring and management
	RegisterAdminEndpoints(r)

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
					bot.Send(msg)
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
					bot.Send(msg)
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
				statusMsgSent, _ := bot.Send(statusMsg)

				err = payment_processing.HandleManualProductDelivery(email, name, product, bot, update.Message.Chat.ID)
				if err != nil {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						fmt.Sprintf("❌ Delivery failed: %v", err))
					bot.Send(msg)
				} else {
					// Edit the previous message to show success
					editMsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, statusMsgSent.MessageID,
						fmt.Sprintf("✅ Product delivered successfully to %s!", email))
					bot.Send(editMsg)
				}
			} else if strings.HasPrefix(message, "/balance") || strings.HasPrefix(message, "!balance") {
				// Handle balance email delivery for USDT
				parts := strings.SplitN(message, " ", 4) // Command, email, name, amount
				if len(parts) < 4 {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						"❌ Invalid format. Use: /balance <email> <n> <amount>")
					bot.Send(msg)
					c.JSON(http.StatusOK, gin.H{"ok": true})
					return
				}

				email := strings.TrimSpace(parts[1])
				name := strings.TrimSpace(parts[2])
				amount := strings.TrimSpace(parts[3])

				// Validate email format
				if !strings.Contains(email, "@") {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "❌ Invalid email format")
					bot.Send(msg)
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
					bot.Send(msg)
					c.JSON(http.StatusOK, gin.H{"ok": true})
					return
				}

				// Format amount with 2 decimal places for consistency
				amount = fmt.Sprintf("%.2f", amountFloat)

				// Send the balance email with progress indication
				statusMsg := tgbotapi.NewMessage(update.Message.Chat.ID,
					fmt.Sprintf("⏳ Sending balance confirmation email to %s...", email))
				statusMsgSent, _ := bot.Send(statusMsg)

				err = payment_processing.HandleManualBalanceEmailDelivery(email, name, amount, bot, update.Message.Chat.ID)
				if err != nil {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						fmt.Sprintf("❌ Balance email failed: %v", err))
					bot.Send(msg)
				} else {
					// Edit the previous message to show success
					editMsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, statusMsgSent.MessageID,
						fmt.Sprintf("✅ Balance confirmation email sent successfully to %s!", email))
					bot.Send(editMsg)
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
				bot.Send(msg)
			}
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	err = r.Run()
	if err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}

	return nil
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
