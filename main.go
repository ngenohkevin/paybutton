package main

import (
	"fmt"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	"github.com/ngenohkevin/paybutton/payments"
	"github.com/ngenohkevin/paybutton/utils"
	"log"
	"net/http"
	"os"
	"time"
)

var (
	botApiKey     string
	chatID        int64 = 6074038462
	addressLimit        = 4              // Limit the number of addresses generated per user/session
	addressExpiry       = 24 * time.Hour // Set address expiry time to 24 hours
)

type UserSession struct {
	Email              string
	GeneratedAddresses map[string]time.Time // Track generated addresses and their creation time
}

var userSessions = make(map[string]*UserSession)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	botApiKey = os.Getenv("BOT_API_KEY")
	if botApiKey == "" {
		log.Fatal("BOT_API_KEY not set in .env file")
	}

	bot, err := tgbotapi.NewBotAPI(botApiKey)
	if err != nil {
		log.Fatal(err)
	}

	r := gin.Default()
	r.Use(cors.Default())

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Payment Service API"})
	})

	r.POST("/cards", handlePayment(bot))
	r.POST("/usdt", handleUsdtPayment(bot))
	r.POST("/payment", handlePayment(bot))
	r.GET("balance/:address", getBalance)

	err = r.Run()
	if err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}

func handlePayment(bot *tgbotapi.BotAPI) gin.HandlerFunc {
	return func(c *gin.Context) {
		processPaymentRequest(c, bot, true)
	}
}

func getBalance(c *gin.Context) {
	address := c.Param("address")

	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Address is required",
		})
		return
	}

	balance, err := payments.GetBitcoinAddressBalance(address)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": fmt.Sprintf("Error fetching balance: %s", err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"address": address,
		"balance": balance,
	})
}

func handleUsdtPayment(bot *tgbotapi.BotAPI) gin.HandlerFunc {
	return func(c *gin.Context) {
		processPaymentRequest(c, bot, false)
	}
}

func processPaymentRequest(c *gin.Context, bot *tgbotapi.BotAPI, generateAddress bool) {
	clientIP := c.ClientIP()
	ipAPIData, err := utils.GetIpLocation(clientIP)
	if err != nil {
		log.Printf("Error getting IP location: %s", err)
	}

	email := c.PostForm("email")
	priceStr := c.PostForm("price")
	description := c.PostForm("description")
	name := c.PostForm("name")

	if email == "" || priceStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid input: email and price are required"})
		return
	}

	priceUSD, err := utils.ParseFloat(priceStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid input: price must be a valid number"})
		return
	}

	// Limit address generation
	session, exists := userSessions[email]
	if !exists {
		session = &UserSession{
			Email:              email,
			GeneratedAddresses: make(map[string]time.Time),
		}
		userSessions[email] = session
	}

	if len(session.GeneratedAddresses) >= addressLimit {
		c.JSON(http.StatusTooManyRequests, gin.H{"message": "Address generation limit reached"})
		return
	}

	var address string
	if generateAddress {
		priceBTC, err := utils.ConvertToBitcoinUSD(priceUSD)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": fmt.Sprintf("Error getting Bitcoin price: %s", err)})
			return
		}
		address, err = payments.GenerateBitcoinAddress(email, priceBTC)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": fmt.Sprintf("Error generating Bitcoin address: %s", err)})
			return
		}
		session.GeneratedAddresses[address] = time.Now()
	} else {
		address = "TMm1VE3JhqDiKyMmizSkcUsx4i4LJkfq7G" // Static USDT address for demonstration
	}

	// Remove expired addresses
	for addr, createdAt := range session.GeneratedAddresses {
		if time.Since(createdAt) > addressExpiry {
			delete(session.GeneratedAddresses, addr)
		}
	}

	localTime, err := ipAPIData.ParseLocalTime()
	if err != nil {
		log.Printf("Error parsing local time: %s", err)
	}

	logMessage := fmt.Sprintf("Email: %s, Address: %s, Amount: %.2f, Name: %s, Product: %s", email, address, priceUSD, name, description)
	log.Printf(logMessage)

	botLogMessage := fmt.Sprintf(
		"*Email:* `%s`\n*Address:* `%s`\n*Amount:* `%0.2f`\n*Name:* `%s`\n*Product:* `%s`\n*IP Address:* `%s`\n*Country:* `%s`\n*State:* `%s`\n*City:* `%s`\n*Local Time:* `%s`",
		email, address, priceUSD, name, description, clientIP, ipAPIData.Location.Country, ipAPIData.Location.State, ipAPIData.Location.City, localTime)

	msg := tgbotapi.NewMessage(chatID, botLogMessage)
	msg.ParseMode = tgbotapi.ModeMarkdown
	_, err = bot.Send(msg)
	if err != nil {
		log.Printf("Error sending message to user: %s", err)
	}

	responseData := gin.H{
		"address":     address,
		"priceInUSD":  priceUSD,
		"email":       email,
		"created_at":  utils.GetCurrentTime(),
		"expired_at":  utils.GetExpiryTime(),
		"description": description,
		"name":        name,
	}

	if generateAddress {
		priceBTC, err := utils.ConvertToBitcoinUSD(priceUSD)
		if err == nil {
			responseData["priceInBTC"] = priceBTC
		}
	}

	c.JSON(http.StatusOK, responseData)
}
