package main

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/ngenohkevin/paybutton/payments"
	"github.com/ngenohkevin/paybutton/utils"
)

var (
	botApiKey         string
	chatID            int64 = 6074038462
	addressLimit            = 4
	addressExpiry           = 24 * time.Hour // Set address expiry time to 24 hours
	blockCypherToken  string
	checkingAddresses = make(map[string]bool)
	db                *sql.DB
	staticBTCAddress  = "bc1qgjnaesfp5k7s8sxz8mq7a3p8rzwpzr3wzp956s"
	staticUSDTAddress = "TMm1VE3JhqDiKyMmizSkcUsx4i4LJkfq7G"
)

type UserSession struct {
	Email                  string
	GeneratedAddresses     map[string]time.Time
	UsedAddresses          map[string]bool
	ExtendedAddressAllowed bool
}

var userSessions = make(map[string]*UserSession)
var mutex sync.Mutex

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	botApiKey = os.Getenv("BOT_API_KEY")
	if botApiKey == "" {
		log.Fatal("BOT_API_KEY not set in .env file")
	}

	blockCypherToken = os.Getenv("BLOCKCYPHER_TOKEN")
	if blockCypherToken == "" {
		log.Fatal("BLOCKCYPHER_TOKEN not set in .env file")
	}

	PostgresUser := os.Getenv("POSTGRES_USER")
	PostgresHost := os.Getenv("POSTGRES_HOST")
	PostgresPassword := os.Getenv("POSTGRES_PASSWORD")
	PostgresDatabase := os.Getenv("POSTGRES_DATABASE")

	db, err = sql.Open("postgres", fmt.Sprintf("user=%s host=%s password=%s dbname=%s sslmode=require", PostgresUser, PostgresHost, PostgresPassword, PostgresDatabase))
	if err != nil {
		log.Fatal("Error connecting to the database:", err)
	}
	defer db.Close()

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
	r.GET("/balance/:address", getBalance)

	err = r.Run()
	if err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}

func handlePayment(bot *tgbotapi.BotAPI) gin.HandlerFunc {
	return func(c *gin.Context) {
		processPaymentRequest(c, bot, true, false)
	}
}

func handleUsdtPayment(bot *tgbotapi.BotAPI) gin.HandlerFunc {
	return func(c *gin.Context) {
		processPaymentRequest(c, bot, false, true)
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

	balance, err := getBitcoinAddressBalanceWithFallback(address, blockCypherToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": fmt.Sprintf("Error fetching balance: %s", err.Error()),
		})
		return
	}
	btc := float64(balance) / 100000000

	rate, err := utils.GetBlockonomicsRate()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": fmt.Sprintf("Error fetching rate: %s", err.Error()),
		})
		return
	}

	balanceUSD := btc * rate
	balanceUSDFormatted := fmt.Sprintf("%.2f", balanceUSD)

	c.JSON(http.StatusOK, gin.H{
		"address": address,
		"balance": balanceUSDFormatted,
	})
}

func processPaymentRequest(c *gin.Context, bot *tgbotapi.BotAPI, generateBtcAddress bool, generateUsdtAddress bool) {
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

	mutex.Lock()
	defer mutex.Unlock()

	// Retrieve or create user session
	session, exists := userSessions[email]
	if !exists {
		session = &UserSession{
			Email:              email,
			GeneratedAddresses: make(map[string]time.Time),
			UsedAddresses:      make(map[string]bool),
		}
		userSessions[email] = session
	}

	var address string
	if generateBtcAddress {
		// Attempt to get a reusable address
		address, err = getReusableAddress(session)
		if (err != nil) || (address == "") {
			// No reusable address found, generate a new one if limit not reached
			addressLimitReached := len(session.GeneratedAddresses) >= addressLimit
			if !addressLimitReached {
				address, err = payments.GenerateBitcoinAddress(email, priceUSD)
				if (err != nil) || (address == "") {
					log.Printf("Error generating Bitcoin address, attempting fallback to static address: %s", err)
					address = fallbackToStaticAddress()
				} else {
					session.GeneratedAddresses[address] = time.Now()
					log.Printf("Generated new address: %s for email: %s", address, email)
					if !checkingAddresses[address] {
						checkingAddresses[address] = true
						go checkBalancePeriodically(address, email, blockCypherToken, bot)
					}
				}
			} else {
				log.Printf("Address generation limit reached for user %s. Reusing address if available.", email)
				address = fallbackToStaticAddress()
			}
		} else {
			log.Printf("Reused address: %s for email: %s", address, email)
			if !checkingAddresses[address] {
				checkingAddresses[address] = true
				go checkBalancePeriodically(address, email, blockCypherToken, bot)
			}
		}
	} else if generateUsdtAddress {
		address = staticUSDTAddress
	} else {
		address = fallbackToStaticAddress()
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

	if generateBtcAddress {
		priceBTC, err := utils.ConvertToBitcoinUSD(priceUSD)
		if err == nil {
			responseData["priceInBTC"] = priceBTC
		}
	}

	c.JSON(http.StatusOK, responseData)
}

func getReusableAddress(session *UserSession) (string, error) {
	for addr, createdAt := range session.GeneratedAddresses {
		// Check if the address is not used and has not expired
		if !session.UsedAddresses[addr] && time.Since(createdAt) <= addressExpiry {
			return addr, nil
		}
	}
	return "", fmt.Errorf("no reusable address found")
}

func fallbackToStaticAddress() string {
	// Log that a fallback is being used
	log.Printf("Using fallback static address")
	return staticBTCAddress
}

func checkBalancePeriodically(address, email, token string, bot *tgbotapi.BotAPI) {
	checkDuration := 20 * time.Minute
	ticker := time.NewTicker(50 * time.Second)
	defer ticker.Stop()
	timeout := time.After(checkDuration)

	for {
		select {
		case <-ticker.C:
			//log.Printf("Checking balance for address %s", address)
			balance, err := getBitcoinAddressBalanceWithFallback(address, token)
			if err != nil {
				log.Printf("Error fetching balance for address %s: %s", address, err)
				continue
			}

			log.Printf("Address: %s, Balance: %d satoshis", address, balance)
			if balance > 0 {
				rate, err := utils.GetBlockonomicsRate()
				if err != nil {
					log.Printf("Error fetching rate: %s", err)
				}

				balanceUSD := float64(balance) / 100000000 * rate
				balanceUSD = roundToTwoDecimalPlaces(balanceUSD)

				var userName string
				err = db.QueryRow("SELECT name FROM users WHERE email = $1", email).Scan(&userName)
				if err != nil {
					log.Printf("Error fetching user name for email %s: %s", email, err)
					continue
				}

				err = updateUserBalance(email, balanceUSD)
				if err != nil {
					log.Printf("Error updating balance for user %s: %s", email, err)
				} else {
					log.Printf("Balance updated successfully for user %s", email)
				}

				mutex.Lock()
				session := userSessions[email]
				session.UsedAddresses[address] = true
				if len(session.UsedAddresses) > 0 && !session.ExtendedAddressAllowed {
					session.ExtendedAddressAllowed = true
				}
				delete(checkingAddresses, address)
				mutex.Unlock()

				confirmationTime := time.Now().Format(time.RFC3339)
				botLogMessage := fmt.Sprintf(
					"*Email:* `%s`\n*New Balance Added:* `%s USD`\n*Confirmation Time:* `%s`",
					email, fmt.Sprintf("%.2f", balanceUSD), confirmationTime)

				msg := tgbotapi.NewMessage(chatID, botLogMessage)
				msg.ParseMode = tgbotapi.ModeMarkdown
				_, err = bot.Send(msg)
				if err != nil {
					log.Printf("Error sending confirmation message to bot: %s", err)
				}

				log.Println("Sending confirmation email to user:", email)
				err = utils.SendEmail(email, userName, fmt.Sprintf("%.2f", balanceUSD))
				if err != nil {
					log.Printf("Error sending email to user %s: %s", email, err)
				} else {
					log.Println("Confirmation email sent successfully to user:", email)
				}

				return
			}

		case <-timeout:
			log.Printf("Stopped checking balance for address %s after %v", address, checkDuration)
			mutex.Lock()
			delete(checkingAddresses, address)
			mutex.Unlock()
			return
		}
	}
}

func getBitcoinAddressBalanceWithFallback(address, token string) (int64, error) {
	balance, err := payments.GetBitcoinAddressBalanceWithBlockonomics(address)
	if err != nil {
		log.Printf("Error with Blockonomics, trying BlockCypher: %s", err)
		balance, err = payments.GetBitcoinAddressBalanceWithBlockCypher(address, token)
		if err != nil {
			log.Printf("Error with BlockCypher, using static address: %s", err)
			balance, err = payments.GetBitcoinAddressBalanceWithBlockonomics(staticBTCAddress)
		}
	}
	return balance, err
}

func updateUserBalance(email string, newBalanceUSD float64) error {
	var currentBalance float64
	err := db.QueryRow("SELECT balance FROM users WHERE email = $1", email).Scan(&currentBalance)
	if err != nil {
		return fmt.Errorf("error fetching current balance for user %s: %w", email, err)
	}

	updatedBalance := roundToTwoDecimalPlaces(currentBalance + newBalanceUSD)

	_, err = db.Exec("UPDATE users SET balance = $1 WHERE email = $2", updatedBalance, email)
	if err != nil {
		return fmt.Errorf("error updating balance for user %s: %w", email, err)
	}

	return nil
}

func roundToTwoDecimalPlaces(value float64) float64 {
	return math.Round(value*100) / 100
}
