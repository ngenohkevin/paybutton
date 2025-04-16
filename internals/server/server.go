package server

import (
	"fmt"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/ngenohkevin/paybutton/internals/payment_processing"
	"github.com/ngenohkevin/paybutton/utils"
	"log"
	"log/slog"
	"net/http"
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
	//r.POST("/notification", handlePaymentNotification(bot))
	//r.GET("/test-email/:email", handleTestEmail)
	//r.GET("/test-product-email/:email/:product", handleTestProductEmail)
	r.GET("/balance/:address", payment_processing.GetBalance)

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

//func handlePaymentNotification(bot *tgbotapi.BotAPI) gin.HandlerFunc {
//	return func(c *gin.Context) {
//		// Extract notification text from the request
//		notification := c.PostForm("notification")
//		if notification == "" {
//			c.JSON(http.StatusBadRequest, gin.H{"error": "Notification text is required"})
//			return
//		}
//
//		// Process the notification
//		err := payment_processing.SendProductFromNotification(notification, bot)
//		if err != nil {
//			// Extract email and product from notification for backup delivery
//			email, _, product, err := utils.ParseNotification(notification)
//			if err == nil && email != "" && product != "" {
//				// Try backup email delivery
//				log.Printf("Attempting backup delivery to %s for %s", email, product)
//				err = utils.SendSimpleTextEmail(
//					email,
//					"Your DWebstore Purchase",
//					fmt.Sprintf("Thank you for your purchase of %s. Your product will be delivered shortly.", product),
//				)
//				if err != nil {
//					log.Printf("Backup delivery also failed: %v", err)
//					c.JSON(http.StatusInternalServerError, gin.H{
//						"error": fmt.Sprintf("Failed to process notification with backup: %v", err),
//					})
//					return
//				}
//				// Backup message sent successfully
//				c.JSON(http.StatusOK, gin.H{
//					"message": "Product delivery notification sent (backup mode)",
//				})
//				return
//			}
//
//			// If we couldn't extract info or backup failed
//			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to process notification: %v", err)})
//			return
//		}
//
//		c.JSON(http.StatusOK, gin.H{"message": "Product delivery initiated successfully"})
//	}
//}
//
//// handleTestEmail is a simple handler to test email functionality
//func handleTestEmail(c *gin.Context) {
//	// Get the email address from the URL parameter
//	email := c.Param("email")
//	if email == "" {
//		c.JSON(http.StatusBadRequest, gin.H{"error": "Email address is required"})
//		return
//	}
//
//	// Try to send a test email
//	log.Printf("Sending test email to %s", email)
//	err := utils.SendTestEmail(email)
//	if err != nil {
//		log.Printf("Failed to send test email: %v", err)
//		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to send test email: %v", err)})
//		return
//	}
//
//	// Report success
//	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Test email sent to %s", email)})
//}
//
//// handleTestProductEmail tests the product delivery email functionality
//func handleTestProductEmail(c *gin.Context) {
//	// Get the email address and product from URL parameters
//	email := c.Param("email")
//	product := c.Param("product")
//
//	// URL decode the product name
//	var err error
//	product, err = url.QueryUnescape(product)
//	if err != nil {
//		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid product name encoding"})
//		return
//	}
//
//	if email == "" {
//		c.JSON(http.StatusBadRequest, gin.H{"error": "Email address is required"})
//		return
//	}
//
//	if product == "" {
//		c.JSON(http.StatusBadRequest, gin.H{"error": "Product name is required"})
//		return
//	}
//
//	// Try each email method until one works
//	log.Printf("Testing product email delivery to %s for %s", email, product)
//
//	// Method 1: Try with normal ProductEmail
//	err = utils.ProductEmail(email, "TestUser", product)
//	if err == nil {
//		c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Product email sent to %s for %s", email, product)})
//		return
//	}
//	log.Printf("Method 1 failed: %v", err)
//
//	// Method 2: Try with backup email
//	err = utils.BackupProductEmail(email, "TestUser", product)
//	if err == nil {
//		c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Backup product email sent to %s for %s", email, product)})
//		return
//	}
//	log.Printf("Method 2 failed: %v", err)
//
//	// Method 3: Try simple text email
//	err = utils.SendSimpleTextEmail(
//		email,
//		fmt.Sprintf("Your DWebstore Purchase: %s", product),
//		fmt.Sprintf("Thank you for your purchase of %s. Your product will be delivered shortly.", product),
//	)
//	if err == nil {
//		c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Simple notification sent to %s for %s", email, product)})
//		return
//	}
//	log.Printf("Method 3 failed: %v", err)
//
//	// Method 4: Try minimal email approach
//	err = payment_processing.MinimalEmailDelivery(email, "TestUser", product)
//	if err == nil {
//		c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Minimal email sent to %s for %s", email, product)})
//		return
//	}
//	log.Printf("Method 4 failed: %v", err)
//
//	// All methods failed
//	c.JSON(http.StatusInternalServerError, gin.H{
//		"error": "All email delivery methods failed",
//		"details": err.Error(),
//	})
//}
