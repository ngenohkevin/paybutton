package payment_processing

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/ngenohkevin/paybutton/utils"
)

var (
	chatID       int64 = 7933331471
	addressLimit       = 6

	// SessionTracker Session tracking callback - set by server package to avoid circular imports
	SessionTracker func(sessionID, address, userAgent, ipAddress, email string, amount float64, paymentID string)
	// SessionStatusUpdater Session status update callback - set by server package to avoid circular imports
	SessionStatusUpdater func(address, status string)
	// SessionWebSocketUpdater WebSocket connection status callback - set by server package to avoid circular imports
	SessionWebSocketUpdater func(address string, connected bool)
	addressExpiry           = 72 * time.Hour // Set address expiry time to 72 hours
	blockCypherToken        string
	blockonomicsAPIKey      string
	checkingAddresses       = make(map[string]bool)
	checkingAddressesTime   = make(map[string]time.Time) // Track when monitoring started
	db                      *sql.DB
	staticBTCAddress        = "bc1q83850augfxlc9wlsj6atdrnsf7nzk8gcuqeecf"
	staticUSDTAddress       = "TBpAXWEGD8LPpx58Fjsu1ejSMJhgDUBNZK"

	// Shared addresses for high-volume periods (different amounts)
	sharedBTCAddresses = map[string]string{
		"tier1": "bc1q83850augfxlc9wlsj6atdrnsf7nzk8gcuqeecf", // $0-50
		"tier2": "bc1q83850augfxlc9wlsj6atdrnsf7nzk8gcuqeecf", // $50-200
		"tier3": "bc1q83850augfxlc9wlsj6atdrnsf7nzk8gcuqeecf", // $200+
	}
)

// InitializeAPIKeys loads API keys from config
func InitializeAPIKeys() error {
	config, err := utils.LoadConfig()
	if err != nil {
		return err
	}
	blockCypherToken = config.BlockCypherToken
	blockonomicsAPIKey = config.BlockonomicsAPI

	// Initialize subsystems
	InitializeAddressPool()
	InitializeRateLimiter()
	InitializeGapMonitor()

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
	GeneratedAddresses     map[string]time.Time              // Keep for backward compatibility
	SiteAddresses         map[string]map[string]time.Time   // NEW: site -> address -> time
	UsedAddresses         map[string]bool
	ExtendedAddressAllowed bool
	PaymentInfo            []PaymentInfo // Store payment information for automatic delivery
	LastActivity           time.Time     // Track last activity for cleanup
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
			LastActivity:       time.Now(),
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
		// Use the new enhanced address generation logic
		address = generateBTCAddressWithEnhancedLogic(email, priceUSD, session, bot, clientIP, site)
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
					checkingAddressesTime[address] = time.Now()
					StartBalanceCheckWithResourceLimit(address, email, blockCypherToken, bot, 60*time.Second)
				}
			} else {
				log.Printf("Address generation limit reached for user %s. Using static USDT address.", email)
				address = staticUSDTAddress
			}
		} else {
			log.Printf("Reused USDT address: %s for email: %s", address, email)
			if !checkingAddresses[address] {
				checkingAddresses[address] = true
				checkingAddressesTime[address] = time.Now()
				StartBalanceCheckWithResourceLimit(address, email, blockCypherToken, bot, 60*time.Second)
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

	// Track session for admin dashboard
	if SessionTracker != nil {
		sessionID := fmt.Sprintf("%s-%d", address, time.Now().Unix())
		userAgent := c.GetHeader("User-Agent")
		paymentID := fmt.Sprintf("pay-%s-%d", strings.ReplaceAll(email, "@", "-"), time.Now().Unix())
		SessionTracker(sessionID, address, userAgent, clientIP, email, priceUSD, paymentID)
	}

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
			LastActivity:           time.Now(),
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
		// Use the enhanced address generation logic for fast mode too
		address = generateBTCAddressWithEnhancedLogic(req.Email, priceUSD, session, bot, clientIP, req.Site)
		mutex.Unlock()

		// For fast mode, always use fast polling if we got a unique address
		if address != staticBTCAddress && !strings.HasPrefix(address, "bc1q83850augfxlc9wlsj6atdrnsf7nzk8gcuqeecf") {
			if !checkingAddresses[address] {
				checkingAddresses[address] = true
				checkingAddressesTime[address] = time.Now()
				StartBalanceCheckWithResourceLimit(address, req.Email, blockCypherToken, bot, 15*time.Second)
			}
		}

		responseData["address"] = address
		// Generate QR code (we don't need the filename for response)
		responseData["qr_code"] = fmt.Sprintf("bitcoin:%s", address)

		// Track session for admin dashboard
		if SessionTracker != nil {
			sessionID := fmt.Sprintf("%s-%d", address, time.Now().Unix())
			userAgent := c.GetHeader("User-Agent")
			paymentID := fmt.Sprintf("fastpay-%s-%d", strings.ReplaceAll(req.Email, "@", "-"), time.Now().Unix())
			SessionTracker(sessionID, address, userAgent, clientIP, req.Email, priceUSD, paymentID)
		}

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

// generateBTCAddressWithEnhancedLogic handles address generation with site-based routing and aggressive reuse
func generateBTCAddressWithEnhancedLogic(email string, priceUSD float64, session *UserSession, bot *tgbotapi.BotAPI, clientIP string, site string) string {
	// Normalize site name
	site = strings.ToLower(site)
	if site == "" {
		log.Printf("No site specified for %s, defaulting to dwebstore", email)
		site = "dwebstore"
	}

	// Validate site exists
	if _, exists := SiteRegistry[site]; !exists {
		log.Printf("Unknown site %s for %s, defaulting to dwebstore", site, email)
		site = "dwebstore"
	}

	// Get site-specific pool
	pool := GetSitePool(site)

	// AGGRESSIVE REUSE: Always try pool first - this prevents gap limit!
	address, err := pool.GetOrReuseAddress(email, priceUSD)
	if err == nil && address != "" {
		// Update session tracking
		if session.SiteAddresses == nil {
			session.SiteAddresses = make(map[string]map[string]time.Time)
		}
		if session.SiteAddresses[site] == nil {
			session.SiteAddresses[site] = make(map[string]time.Time)
		}
		session.SiteAddresses[site][address] = time.Now()

		// Also update legacy tracking for backward compatibility
		session.GeneratedAddresses[address] = time.Now()

		// Start monitoring
		if !checkingAddresses[address] {
			checkingAddresses[address] = true
			checkingAddressesTime[address] = time.Now()
			StartBalanceCheckWithResourceLimit(address, email, blockCypherToken, bot, 60*time.Second)
		}

		log.Printf("Address %s assigned to %s for site %s", address, email, site)
		return address
	}

	// If pool is exhausted, try to generate more
	log.Printf("Pool exhausted for %s, attempting emergency generation", site)
	if err := PreGenerateAddressPool(site, 10); err != nil {
		log.Printf("CRITICAL: Cannot generate addresses for %s: %v", site, err)
		// Use shared fallback as absolute last resort
		return getSharedAddressForAmount(priceUSD)
	}

	// Try pool again
	address, err = pool.GetOrReuseAddress(email, priceUSD)
	if err != nil {
		log.Printf("CRITICAL: Still cannot get address for %s on %s", email, site)
		return getSharedAddressForAmount(priceUSD)
	}

	// Update session tracking
	if session.SiteAddresses == nil {
		session.SiteAddresses = make(map[string]map[string]time.Time)
	}
	if session.SiteAddresses[site] == nil {
		session.SiteAddresses[site] = make(map[string]time.Time)
	}
	session.SiteAddresses[site][address] = time.Now()
	session.GeneratedAddresses[address] = time.Now()

	// Start monitoring
	if !checkingAddresses[address] {
		checkingAddresses[address] = true
		checkingAddressesTime[address] = time.Now()
		StartBalanceCheckWithResourceLimit(address, email, blockCypherToken, bot, 60*time.Second)
	}

	log.Printf("Address %s assigned to %s for site %s (after emergency generation)", address, email, site)
	return address
}

// getSharedAddressForAmount returns a shared address based on amount tier
func getSharedAddressForAmount(amount float64) string {
	if amount <= 50 {
		return sharedBTCAddresses["tier1"]
	} else if amount <= 200 {
		return sharedBTCAddresses["tier2"]
	}
	return sharedBTCAddresses["tier3"]
}

// isGapLimitError checks if the error is related to gap limit
func isGapLimitError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "Gap Limit") ||
		strings.Contains(errStr, "gap limit") ||
		strings.Contains(errStr, "code: 1008") ||
		strings.Contains(errStr, "too many addresses")
}

func getReusableAddress(session *UserSession, currencyType string) (string, error) {
	// First check session addresses
	for addr, createdAt := range session.GeneratedAddresses {
		// Skip if it's not the requested currency type
		if currencyType == "BTC" && !btcRegex.MatchString(addr) {
			continue
		}
		if currencyType == "USDT" && !usdtRegex.MatchString(addr) {
			continue
		}

		// IMPORTANT: Reuse unpaid addresses to avoid gap limit issues
		// Check if the address is not used (not paid) and still within expiry
		if !session.UsedAddresses[addr] && time.Since(createdAt) <= addressExpiry {
			log.Printf("Reusing unpaid %s address %s from session (age: %v) - Gap limit optimization", 
				currencyType, addr, time.Since(createdAt).Round(time.Minute))
			return addr, nil
		}
	}
	
	// If BTC and no session address available, try to get from address pool
	// This helps reuse unpaid addresses from the pool system
	if currencyType == "BTC" {
		pool := GetAddressPool()
		if pool != nil {
			// Try to get an existing reserved address for this user
			if addr, err := pool.ReserveAddress(session.Email, 0); err == nil && addr != "" {
				// Add to session for tracking
				session.GeneratedAddresses[addr] = time.Now()
				log.Printf("Reusing address %s from pool for %s - Gap limit optimization", addr, session.Email)
				return addr, nil
			}
		}
	}
	
	return "", fmt.Errorf("no reusable %s address found", currencyType)
}
