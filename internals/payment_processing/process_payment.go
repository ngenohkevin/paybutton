package payment_processing

import (
	"database/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	payments2 "github.com/ngenohkevin/paybutton/internals/payments"
	"github.com/ngenohkevin/paybutton/utils"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	chatID             int64 = 7933331471
	addressLimit             = 6
	addressExpiry            = 72 * time.Hour // Set address expiry time to 72 hours
	blockCypherToken   string
	blockonomicsAPIKey string
	checkingAddresses  = make(map[string]bool)
	db                 *sql.DB
	staticBTCAddress   = "bc1q7ss2m46955mps6sytsmmjl73hz5v6etprvjsms"
	staticUSDTAddress  = "TBpAXWEGD8LPpx58Fjsu1ejSMJhgDUBNZK"
)

// InitializeAPIKeys loads API keys from config
func InitializeAPIKeys() error {
	config, err := utils.LoadConfig()
	if err != nil {
		return err
	}
	blockCypherToken = config.BlockCypherToken
	blockonomicsAPIKey = config.BlockonomicsAPI
	return nil
}

// GetChatID returns the chat ID for Telegram notifications
func GetChatID() int64 {
	return chatID
}

type PaymentInfo struct {
	Price       float64
	Description string
	Name        string
	Site        string
	CreatedAt   time.Time
}

type UserSession struct {
	Email                  string
	GeneratedAddresses     map[string]time.Time
	UsedAddresses          map[string]bool
	ExtendedAddressAllowed bool
	PaymentInfo            []PaymentInfo // Store payment information for automatic delivery
}

var userSessions = make(map[string]*UserSession)
var mutex sync.Mutex

func ProcessPaymentRequest(c *gin.Context, bot *tgbotapi.BotAPI, generateBtcAddress bool, generateUsdtAddress bool) {
	clientIP := c.ClientIP()
	ipAPIData, err := utils.GetIpLocation(clientIP)
	if err != nil {
		log.Printf("Error getting IP location: %s", err)

		// Proceed with default or partial data
		ipAPIData = &utils.IPAPIData{
			Location: utils.IPAPILocation{
				Continent: "Unknown",
				Country:   "Unknown",
				City:      "Unknown",
				Timezone:  "UTC",
			},
		}
	}

	localTime, err := ipAPIData.ParseLocalTime()
	if err != nil {
		log.Printf("Error parsing local time: %s", err)
		localTime = "00:00" // Default time
	}

	log.Printf("Client IP: %s, Local Time: %s", clientIP, localTime)

	email := c.PostForm("email")
	priceStr := c.PostForm("price")
	description := c.PostForm("description")
	name := c.PostForm("name")
	site := c.PostForm("site")

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
			PaymentInfo:        []PaymentInfo{},
		}
		userSessions[email] = session
	}

	// Store the payment information for future use
	paymentInfo := PaymentInfo{
		Price:       priceUSD,
		Description: description,
		Name:        name,
		Site:        site,
		CreatedAt:   time.Now(),
	}
	session.PaymentInfo = append(session.PaymentInfo, paymentInfo)

	var address string
	if generateBtcAddress {
		// Attempt to get a reusable BTC address
		address, err = getReusableAddress(session, "BTC")
		if err != nil || address == "" {
			// No reusable address found, generate a new one if limit not reached
			addressLimitReached := len(session.GeneratedAddresses) >= addressLimit
			if addressLimitReached {
				// Check if any address has received balance to extend the limit
				if session.ExtendedAddressAllowed {
					addressLimitReached = false
				} else {
					for addr := range session.GeneratedAddresses {
						if session.UsedAddresses[addr] {
							addressLimitReached = false
							break
						}
					}
				}
			}

			if !addressLimitReached {
				address, err = payments2.GenerateBitcoinAddress(email, priceUSD)
				if err != nil || address == "" {
					log.Printf("Error generating Bitcoin address, attempting fallback to static address: %s", err)
					address = staticBTCAddress
				} else {
					session.GeneratedAddresses[address] = time.Now()
					log.Printf("Generated new BTC address: %s for email: %s", address, email)
					if !checkingAddresses[address] {
						checkingAddresses[address] = true
						go checkBalancePeriodically(address, email, blockCypherToken, bot)
					}
				}
			} else {
				log.Printf("Address generation limit reached for user %s. Using static BTC address.", email)
				address = staticBTCAddress
			}
		} else {
			log.Printf("Reused BTC address: %s for email: %s", address, email)
			if !checkingAddresses[address] {
				checkingAddresses[address] = true
				go checkBalancePeriodically(address, email, blockCypherToken, bot)
			}
		}
	} else if generateUsdtAddress {
		// Attempt to get a reusable USDT address
		address, err = getReusableAddress(session, "USDT")
		if err != nil || address == "" {
			// No reusable address found, generate a new one if limit not reached
			addressLimitReached := len(session.GeneratedAddresses) >= addressLimit
			if addressLimitReached {
				// Check if any address has received balance to extend the limit
				if session.ExtendedAddressAllowed {
					addressLimitReached = false
				} else {
					for addr := range session.GeneratedAddresses {
						if session.UsedAddresses[addr] {
							addressLimitReached = false
							break
						}
					}
				}
			}

			if !addressLimitReached {
				address = utils.RandomUSDTAddress()

				// Verify the USDT address format
				if !usdtRegex.MatchString(address) {
					log.Printf("WARNING: Generated USDT address does not match the expected format: %s", address)
					// Still continue, but with a warning
				}

				session.GeneratedAddresses[address] = time.Now()
				log.Printf("Generated new USDT address: %s for email: %s", address, email)
				if !checkingAddresses[address] {
					checkingAddresses[address] = true
					go checkBalancePeriodically(address, email, blockCypherToken, bot)
				}
			} else {
				log.Printf("Address generation limit reached for user %s. Using static USDT address.", email)
				address = staticUSDTAddress
			}
		} else {
			log.Printf("Reused USDT address: %s for email: %s", address, email)
			if !checkingAddresses[address] {
				checkingAddresses[address] = true
				go checkBalancePeriodically(address, email, blockCypherToken, bot)
			}
		}
	} else {
		address = staticBTCAddress
	}

	// Remove expired addresses
	for addr, createdAt := range session.GeneratedAddresses {
		if time.Since(createdAt) > addressExpiry {
			delete(session.GeneratedAddresses, addr)
		}
	}

	localTime, err = ipAPIData.ParseLocalTime()
	if err != nil {
		log.Printf("Error parsing local time: %s", err)
	}

	logMessage := fmt.Sprintf("Email: %s, Address: %s, Amount: %.2f, Name: %s, Product: %s", email, address, priceUSD, name, description)
	log.Printf(logMessage)

	botLogMessage := fmt.Sprintf(
		"*Site:* `%s`\n*Email:* `%s`\n*Address:* `%s`\n*Amount:* `%0.2f`\n*Name:* `%s`\n*Product:* `%s`\n*IP Address:* `%s`\n*Country:* `%s`\n*State:* `%s`\n*City:* `%s`\n*Local Time:* `%s`",
		site, email, address, priceUSD, name, description, clientIP, ipAPIData.Location.Country, ipAPIData.Location.State, ipAPIData.Location.City, localTime)

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
		"site":        site,
	}

	if generateBtcAddress {
		priceBTC, err := utils.ConvertToBitcoinUSD(priceUSD)
		if err == nil {
			responseData["priceInBTC"] = priceBTC
		}
	} else if generateUsdtAddress {
		// For USDT, the price in USDT is the same as USD (1:1 peg)
		responseData["priceInUSDT"] = priceUSD
		responseData["currency"] = "USDT (TRC20)"
	}

	c.JSON(http.StatusOK, responseData)
}

// ProcessFastPaymentRequest - Enhanced version with 15-second polling for faster notifications
func ProcessFastPaymentRequest(c *gin.Context, bot *tgbotapi.BotAPI, generateBtcAddress bool, generateUsdtAddress bool) {
	clientIP := c.ClientIP()
	ipAPIData, err := utils.GetIpLocation(clientIP)
	if err != nil {
		log.Printf("Error getting IP location: %s", err)

		// Proceed with default or partial data
		ipAPIData = &utils.IPAPIData{
			Location: utils.IPAPILocation{
				City:      "Unknown",
				State:     "Unknown",
				Country:   "Unknown",
				Continent: "Unknown",
			},
		}
	}

	log.Printf("Request IP: %s, Location: %s, %s, %s", clientIP, ipAPIData.Location.City, ipAPIData.Location.State, ipAPIData.Location.Country)

	var req struct {
		Email       string  `json:"email" binding:"required"`
		Price       float64 `json:"price" binding:"required"`
		Description string  `json:"description" binding:"required"`
		Name        string  `json:"name" binding:"required"`
		Site        string  `json:"site"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request data",
			"error":   err.Error(),
		})
		return
	}

	if req.Price <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Price must be greater than zero",
		})
		return
	}

	mutex.Lock()
	session := userSessions[req.Email]
	if session == nil {
		session = &UserSession{
			Email:                  req.Email,
			GeneratedAddresses:     make(map[string]time.Time),
			UsedAddresses:          make(map[string]bool),
			ExtendedAddressAllowed: false,
			PaymentInfo:            []PaymentInfo{},
		}
		userSessions[req.Email] = session
	}

	// Store payment information for automatic delivery
	paymentInfo := PaymentInfo{
		Price:       req.Price,
		Description: req.Description,
		Name:        req.Name,
		Site:        req.Site,
		CreatedAt:   time.Now(),
	}
	session.PaymentInfo = append(session.PaymentInfo, paymentInfo)
	mutex.Unlock()

	priceUSD := req.Price

	responseData := gin.H{
		"message":     "Payment request processed with fast polling",
		"email":       req.Email,
		"price":       req.Price,
		"description": req.Description,
		"name":        req.Name,
		"site":        req.Site,
		"currency":    "BTC",
		"polling":     "15s",
	}

	var address string
	if generateBtcAddress {
		mutex.Lock()
		reusableAddr, err := getReusableAddress(session, "BTC")
		if err != nil {
			if len(session.GeneratedAddresses) < addressLimit || session.ExtendedAddressAllowed {
				address, err = payments2.GenerateBitcoinAddress(req.Email, priceUSD)
				if err != nil {
					mutex.Unlock()
					c.JSON(http.StatusInternalServerError, gin.H{
						"message": "Error generating Bitcoin address",
						"error":   err.Error(),
					})
					return
				}
				session.GeneratedAddresses[address] = time.Now()
				log.Printf("Generated new BTC address: %s for email: %s (FAST MODE)", address, req.Email)
				if !checkingAddresses[address] {
					checkingAddresses[address] = true
					go CheckBalanceFast(address, req.Email, blockCypherToken, bot)
				}
			} else {
				log.Printf("Address generation limit reached for user %s. Using static BTC address. (FAST MODE)", req.Email)
				address = staticBTCAddress
			}
		} else {
			address = reusableAddr
			log.Printf("Reused BTC address: %s for email: %s (FAST MODE)", address, req.Email)
			if !checkingAddresses[address] {
				checkingAddresses[address] = true
				go CheckBalanceFast(address, req.Email, blockCypherToken, bot)
			}
		}
		mutex.Unlock()

		responseData["address"] = address
		// Generate QR code (we don't need the filename for response)
		responseData["qr_code"] = fmt.Sprintf("bitcoin:%s", address)

		priceBTC, err := utils.ConvertToBitcoinUSD(priceUSD)
		if err == nil {
			responseData["priceInBTC"] = priceBTC
		}

		// Log the complete request with address to bot
		locationStr := ipAPIData.Location.City
		if ipAPIData.Location.State != "" && ipAPIData.Location.State != ipAPIData.Location.City {
			locationStr = fmt.Sprintf("%s, %s", ipAPIData.Location.City, ipAPIData.Location.State)
		}
		if ipAPIData.Location.Country != "" {
			locationStr = fmt.Sprintf("%s, %s", locationStr, ipAPIData.Location.Country)
		}

		// Get proper local time for the notification
		localTime, err := ipAPIData.ParseLocalTime()
		if err != nil {
			log.Printf("Error parsing local time: %s", err)
			localTime = "00:00"
		}

		logMessage := fmt.Sprintf(
			"💰 *Fast Payment Request*\n\n"+
				"*Site:* `%s`\n"+
				"*Email:* `%s`\n"+
				"*Address:* `%s`\n"+
				"*Amount:* `$%.2f`\n"+
				"*Name:* `%s`\n"+
				"*Product:* `%s`\n"+
				"*IP Address:* `%s`\n"+
				"*Country:* `%s`\n"+
				"*State:* `%s`\n"+
				"*City:* `%s`\n"+
				"*Local Time:* `%s`\n\n"+
				"*Mode: Fast Polling (15s)*",
			req.Site, req.Email, address, priceUSD, req.Name, req.Description,
			clientIP, ipAPIData.Location.Country, ipAPIData.Location.State, ipAPIData.Location.City, localTime)

		msg := tgbotapi.NewMessage(chatID, logMessage)
		msg.ParseMode = tgbotapi.ModeMarkdown
		_, err = bot.Send(msg)
		if err != nil {
			log.Printf("Error sending log message to bot: %s", err)
		}
	}

	c.JSON(http.StatusOK, responseData)
}

func getReusableAddress(session *UserSession, currencyType string) (string, error) {
	for addr, createdAt := range session.GeneratedAddresses {
		// Skip if it's not the requested currency type
		if currencyType == "BTC" && !btcRegex.MatchString(addr) {
			continue
		}
		if currencyType == "USDT" && !usdtRegex.MatchString(addr) {
			continue
		}

		// Check if the address is not used and has not expired
		if !session.UsedAddresses[addr] && time.Since(createdAt) <= addressExpiry {
			return addr, nil
		}
	}
	return "", fmt.Errorf("no reusable %s address found", currencyType)
}
