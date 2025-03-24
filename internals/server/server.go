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

	//define the router
	r := gin.Default()
	r.Use(cors.Default())

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Payment Service API"})
	})

	r.POST("/cards", handlePayment(bot))
	r.POST("/usdt", handleUsdtPayment(bot))
	r.POST("/payment", handlePayment(bot))
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
