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

	bot, err := tgbotapi.NewBotAPI(botToken.BotApiKey)
	if err != nil {
		s.logger.Error("Error initializing bot:", err)
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
	r.GET("/balance/:address", payment_processing.GetBalance)

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

			// Check if this is a delivery command
			if strings.HasPrefix(message, "/deliver") || strings.HasPrefix(message, "!deliver") {
				// Extract the notification text from the command
				parts := strings.SplitN(message, " ", 2)
				if len(parts) < 2 {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "❌ Invalid format. Use: /deliver <notification>")
					bot.Send(msg)
					c.JSON(http.StatusOK, gin.H{"ok": true})
					return
				}

				notification := parts[1]

				// Parse the notification to extract details
				email, name, product, err := utils.ParseNotification(notification)
				if err != nil {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						fmt.Sprintf("❌ Failed to parse notification: %v", err))
					bot.Send(msg)
					c.JSON(http.StatusOK, gin.H{"ok": true})
					return
				}

				// Use default name if empty
				if name == "" {
					name = "Customer"
				}

				// Send the product email
				chatID := payment_processing.GetChatID() // Implement this function to expose the chatID
				err = utils.ProductEmail(email, name, product)
				if err != nil {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						fmt.Sprintf("❌ Delivery failed: %v", err))
					bot.Send(msg)

					// Notify telegram of failure
					failMsg := utils.BuildProductPayloadForBot(email, name, product, "failed")
					failNotif := tgbotapi.NewMessage(chatID, failMsg)
					failNotif.ParseMode = tgbotapi.ModeMarkdown
					_, _ = bot.Send(failNotif)
				} else {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "✅ Product delivered successfully!")
					bot.Send(msg)

					// Notify telegram of success
					successMsg := utils.BuildProductPayloadForBot(email, name, product, "delivery")
					successNotif := tgbotapi.NewMessage(chatID, successMsg)
					successNotif.ParseMode = tgbotapi.ModeMarkdown
					_, _ = bot.Send(successNotif)
				}
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
