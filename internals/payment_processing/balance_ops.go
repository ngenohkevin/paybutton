package payment_processing

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/ngenohkevin/paybutton/internals/database"
	dbgen "github.com/ngenohkevin/paybutton/internals/db"
	payments2 "github.com/ngenohkevin/paybutton/internals/payments"
	"github.com/ngenohkevin/paybutton/utils"
)

// Regular expressions for Bitcoin and USDT addresses
var btcRegex = regexp.MustCompile(`^(1|3)[A-HJ-NP-Za-km-z1-9]{25,34}$|^bc1[qpzry9x8gf2tvdw0s3jn54khce6mua7l]{39,59}$`)
var usdtRegex = regexp.MustCompile(`^T[a-zA-HJ-NP-Z0-9]{33}$`)

func GetBalance(c *gin.Context) {
	address := c.Param("address")

	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Address is required",
		})
		return
	}

	if btcRegex.MatchString(address) {
		// Handle BTC balance
		balance, err := GetBitcoinAddressBalanceWithFallback(address, blockCypherToken)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": fmt.Sprintf("Error fetching BTC balance: %s", err.Error()),
			})
			return
		}
		btc := float64(balance) / 100000000 // Convert satoshis to BTC

		rate, err := utils.GetBlockonomicsRate()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": fmt.Sprintf("Error fetching BTC rate: %s", err.Error()),
			})
			return
		}

		balanceUSD := btc * rate
		balanceUSDFormatted := fmt.Sprintf("%.2f", balanceUSD)

		c.JSON(http.StatusOK, gin.H{
			"address":     address,
			"currency":    "BTC",
			"balance":     balanceUSDFormatted,
			"raw_balance": fmt.Sprintf("%.8f", btc),
			"rate":        rate,
		})
	} else if usdtRegex.MatchString(address) {
		// Handle USDT balance (manual checking still works)
		balance, err := payments2.GetUSDTBalance(address)
		if err != nil {
			// Try one more time before returning error
			time.Sleep(2 * time.Second)
			balance, err = payments2.GetUSDTBalance(address)

			// Even if there's still an error, if it's just a "no tokens" or API error,
			// we'll return 0 balance instead of an error
			if err != nil {
				// Check if it's a DNS or connection error (which is a real error)
				if strings.Contains(err.Error(), "dial tcp") ||
					strings.Contains(err.Error(), "no such host") {
					// Return a friendlier error message
					c.JSON(http.StatusServiceUnavailable, gin.H{
						"message":     "Service temporarily unavailable. Please try again later.",
						"address":     address,
						"currency":    "USDT (TRC20)",
						"balance":     "0.00",
						"raw_balance": "0.000000",
						"rate":        1.0,
					})
					return
				}

				// For other errors, assume it's just a zero balance
				balance = 0
			}
		}

		// USDT is already in USD equivalent
		balanceUSDFormatted := fmt.Sprintf("%.2f", balance)

		// Check if balance is actually available or zero
		if balance <= 0 {
			// No logging needed for zero balance - common case
		}

		c.JSON(http.StatusOK, gin.H{
			"address":     address,
			"currency":    "USDT (TRC20)",
			"balance":     balanceUSDFormatted,
			"raw_balance": fmt.Sprintf("%.6f", balance),
			"rate":        1.0, // USDT is pegged to USD at 1:1
		})
	} else {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid address format",
		})
	}
}

// GetProductFromDescription attempts to extract a product name from the payment description
func GetProductFromDescription(description string) string {
	// This function helps in cases where the product name might be stored in different formats
	// or needs to be normalized from the payment description

	// Check if description is empty
	if description == "" {
		return "Unknown Product"
	}

	// If description contains "Product:" extract the product
	if strings.Contains(description, "Product:") {
		parts := strings.Split(description, "Product:")
		if len(parts) > 1 {
			return strings.TrimSpace(parts[1])
		}
	}

	// If no specific product info found, return the entire description as the product
	return description
}

// Original function - kept for backward compatibility
func checkBalancePeriodically(address, email, token string, bot *tgbotapi.BotAPI) {
	checkBalanceWithInterval(address, email, token, bot, 60*time.Second)
}

// Enhanced function with configurable polling interval
func CheckBalanceFast(address, email, token string, bot *tgbotapi.BotAPI) {
	checkBalanceWithInterval(address, email, token, bot, 15*time.Second)
}

// Enhanced function with configurable polling interval
func checkBalanceWithInterval(address, email, token string, bot *tgbotapi.BotAPI, interval time.Duration) {
	checkDuration := 30 * time.Minute
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	timeout := time.After(checkDuration)

	var currencyType string
	if btcRegex.MatchString(address) {
		currencyType = "BTC"
	} else if usdtRegex.MatchString(address) {
		currencyType = "USDT"
	} else {
		log.Printf("Unknown address format for %s, defaulting to BTC", address)
		currencyType = "BTC"
	}

	// TEMPORARY: Disable USDT balance checking
	if currencyType == "USDT" {
		log.Printf("USDT balance checking is temporarily disabled for address: %s (email: %s)", address, email)
		return
	}

	log.Printf("Starting balance check for %s address: %s", currencyType, address)

	for {
		select {
		case <-ticker.C:
			var internalBalance float64
			var balance float64
			var err error

			var satoshis int64 // Declare satoshis in function scope
			if currencyType == "BTC" {
				// BTC balance check
				satoshis, err = GetBitcoinAddressBalanceWithFallback(address, token)
				if err != nil {
					log.Printf("Error fetching BTC balance for address %s: %s", address, err)
					continue
				}

				// Convert satoshis to BTC, then to USD
				btc := float64(satoshis) / 100000000
				rate, err := utils.GetBlockonomicsRate()
				if err != nil {
					log.Printf("Error fetching BTC rate: %s", err)
					continue
				}
				balance = btc * rate
				log.Printf("Checking %s: %.8f BTC (%.2f USD)", address, btc, balance)
			} else {
				// USDT balance check - USDT value is already in USD
				usdtBalance, err := payments2.GetUSDTBalance(address)
				if err != nil {
					log.Printf("Error fetching USDT balance for address %s: %s", address, err)
					continue
				}

				// USDT is already in USD equivalent (1 USDT ≈ 1 USD)
				internalBalance = usdtBalance

				// For USDT, only send notification to bot about balance
				if internalBalance > 0 {
					confirmationTime := time.Now().Format(time.RFC3339)
					botLogMessage := fmt.Sprintf(
						"*Email:* `%s`\n*USDT Balance Detected (%s):* `%s USD`\n*Confirmation Time:* `%s`",
						email, currencyType, fmt.Sprintf("%.2f", usdtBalance), confirmationTime)

					msg := tgbotapi.NewMessage(chatID, botLogMessage)
					msg.ParseMode = tgbotapi.ModeMarkdown
					_, err = bot.Send(msg)
					if err != nil {
						log.Printf("Error sending confirmation message to bot: %s", err)
					}

					// Mark address as used but without updating DB or sending emails
					mutex.Lock()
					session := userSessions[email]
					if session != nil {
						session.UsedAddresses[address] = true
						if len(session.UsedAddresses) > 0 && !session.ExtendedAddressAllowed {
							session.ExtendedAddressAllowed = true
						}
					} else {
						log.Printf("Warning: User session for %s not found when marking address %s as used", email, address)
					}
					delete(checkingAddresses, address)
					mutex.Unlock()

					// Update gap monitor for USDT payment
					gapMonitor := GetGapMonitor()
					gapMonitor.RecordPayment(address)

					// Update address pool if applicable
					addressPool := GetAddressPool()
					addressPool.MarkAddressUsed(address, email)

					// Exit the check cycle as we've detected USDT
					return
				}

				// Log different messages based on balance
				if internalBalance <= 0 {
					log.Printf("Checking %s: No USDT balance", address)
				} else {
					log.Printf("Checking %s: %.6f USDT (%.2f USD)", address, usdtBalance, internalBalance)
				}
			}

			if balance > 0 && currencyType == "BTC" {
				// Check if this is a static/shared address - if so, only notify admin
				// because the balance could be from any user, not necessarily this one
				if isStaticOrSharedAddress(address) {
					log.Printf("Balance detected on static/shared address %s (monitoring for %s): %.2f USD", address, email, balance)

					// Send notification to admin for manual verification (like USDT)
					confirmationTime := time.Now().Format(time.RFC3339)
					botLogMessage := fmt.Sprintf(
						"*Email:* `%s`\n*BTC Balance Detected (Static Address):* `%.2f USD`\n*Address:* `%s`\n*Confirmation Time:* `%s`\n\n⚠️ *Static address - manual verification required*",
						email, balance, address, confirmationTime)

					msg := tgbotapi.NewMessage(chatID, botLogMessage)
					msg.ParseMode = tgbotapi.ModeMarkdown
					_, err = bot.Send(msg)
					if err != nil {
						log.Printf("Error sending static address balance notification: %s", err)
					}

					// Mark address as detected but don't process automatically
					mutex.Lock()
					session := userSessions[email]
					if session != nil {
						// Mark that we've seen balance on this address
						session.UsedAddresses[address] = true
					}
					delete(checkingAddresses, address)
					mutex.Unlock()

					// Exit monitoring for this address
					return
				}

				balanceUSD := database.RoundToTwoDecimalPlaces(balance)

				// Calculate BTC amount for WebSocket notification
				btcAmount := float64(satoshis) / 100000000 // Convert satoshis back to BTC

				// Send WebSocket and SSE notification immediately
				BroadcastBalanceUpdateAll(address, "confirmed", balanceUSD, btcAmount, email)

				// Update session status to completed via callback if available
				if SessionStatusUpdater != nil {
					SessionStatusUpdater(address, "completed")
				}

				// Update payment record in database - create persistence instance for entire flow
				paymentPersistence := NewPaymentPersistence()
				var currentPayment *dbgen.Payment
				if paymentPersistence.IsEnabled() {
					ctx := context.Background()
					payment, err := paymentPersistence.GetPaymentByAddress(ctx, address)
					if err == nil && payment != nil {
						currentPayment = payment

						// CRITICAL: Fetch transaction hash when balance detected
						// Webhook has txid automatically, but balance polling needs to fetch it
						txHash := ""
						if txHashFetched, err := payments2.GetRecentTransactionHash(address); err == nil {
							txHash = txHashFetched
							log.Printf("✅ Fetched transaction hash for %s: %s", address, txHash)
						} else {
							log.Printf("⚠️  Could not fetch transaction hash for %s: %v (will update without txid)", address, err)
						}

						// Update with transaction details (confirmations = 1 for BTC balance detected)
						_ = paymentPersistence.UpdatePaymentTransaction(ctx, payment.PaymentID, txHash, 1)
						_ = paymentPersistence.UpdatePaymentConfirmed(ctx, payment.PaymentID, 1)
						log.Printf("✅ Updated payment record %s: confirmed (tx: %s)", payment.PaymentID, txHash)
					}
				}

				// Try to get username from the database, but don't fail if not found
				var userName string
				err = database.DB.QueryRow("SELECT name FROM users WHERE email = $1", email).Scan(&userName)
				if err != nil {
					log.Printf("User with email %s not found in database: %s", email, err)
					userName = "User" // Set default name
				}

				// CRITICAL FIX: Always use payment record site, NOT address mapping
				// Address mapping can be wrong when addresses are reassigned across sites via global pool
				site := ""
				if currentPayment != nil {
					site = currentPayment.Site
					log.Printf("Using site '%s' from payment record for address %s", site, address)
				} else {
					// Fallback: try address mapping (can be inaccurate for global pool addresses)
					site = GetSiteForAddress(address)
					if site == "" {
						// Last resort: check session
						mutex.Lock()
						if session, exists := userSessions[email]; exists && len(session.PaymentInfo) > 0 {
							latestPayment := session.PaymentInfo[len(session.PaymentInfo)-1]
							site = latestPayment.Site
						}
						mutex.Unlock()
					}
					if site != "" {
						log.Printf("⚠️  WARNING: Using fallback site '%s' for address %s (no payment record found)", site, address)
					}
				}

				// Get site configuration
				siteConfig, exists := SiteRegistry[strings.ToLower(site)]
				if !exists {
					log.Printf("Unknown site %s for payment, defaulting to dwebstore", site)
					siteConfig = SiteRegistry["dwebstore"]
				}

				productName := ""
				mutex.Lock()
				if session, exists := userSessions[email]; exists && len(session.PaymentInfo) > 0 {
					latestPayment := session.PaymentInfo[len(session.PaymentInfo)-1]
					productName = latestPayment.Description
				}
				mutex.Unlock()

				// Mark address as used and update monitoring systems
				mutex.Lock()
				if session, exists := userSessions[email]; exists && session != nil {
					session.UsedAddresses[address] = true
					if len(session.UsedAddresses) > 0 && !session.ExtendedAddressAllowed {
						session.ExtendedAddressAllowed = true
					}
				} else {
					log.Printf("Warning: User session for %s not found when marking address %s as used", email, address)
				}
				delete(checkingAddresses, address)
				mutex.Unlock()

				// Update gap monitor - payment received reduces unpaid count
				gapMonitor := GetGapMonitor()
				gapMonitor.RecordPayment(address)

				// Mark address as used in site-specific pool
				pool := GetSitePool(strings.ToLower(siteConfig.Name))
				pool.MarkAddressUsed(address)

				// Also update legacy address pool for backward compatibility
				addressPool := GetAddressPool()
				addressPool.MarkAddressUsed(address, email)

				confirmationTime := time.Now().Format(time.RFC3339)

				// Site-based routing using configuration
				switch siteConfig.Type {
				case SiteTypeProductDelivery:
					// Dwebstore - product delivery
					log.Printf("%s payment detected - processing product delivery for %s: %s",
						siteConfig.Name, email, productName)

					if productName != "" {
						err = HandleAutomaticDelivery(email, userName, productName, siteConfig.Name, bot)
						if err != nil {
							log.Printf("Error in automatic product delivery: %s", err)
						} else {
							log.Printf("Automatic product delivery successful for %s", email)
						}
					} else {
						log.Printf("Skipping product delivery for %s as product not found in session", email)
					}

					// Send product delivery confirmation email
					emailSent := false
					if userName != "" && userName != "User" {
						log.Println("📧 Sending product delivery email to user:", email)
						err = utils.SendEmail(email, userName, fmt.Sprintf("%.2f", balanceUSD))
						if err != nil {
							log.Printf("Error sending product delivery email to user %s: %s", email, err)
						} else {
							log.Println("✅ Product delivery email sent successfully to user:", email)
							emailSent = true
						}
					} else {
						log.Printf("Skipping email send for %s - user name not available", email)
					}

					// Telegram notification for product delivery
					botLogMessage := fmt.Sprintf(
						"*Email:* `%s`\n*Product Delivered (%s):* `%s`\n*Amount:* `%s USD`\n*Site:* `%s`\n*Confirmation Time:* `%s`",
						email, currencyType, productName, fmt.Sprintf("%.2f", balanceUSD), siteConfig.Name, confirmationTime)

					msg := tgbotapi.NewMessage(chatID, botLogMessage)
					msg.ParseMode = tgbotapi.ModeMarkdown
					telegramSent := false
					_, err = bot.Send(msg)
					if err != nil {
						log.Printf("Error sending product delivery confirmation to bot: %s", err)
					} else {
						telegramSent = true
					}

					// Mark notifications sent in payment record
					if paymentPersistence.IsEnabled() && currentPayment != nil {
						ctx := context.Background()
						if emailSent {
							err := paymentPersistence.MarkEmailSent(ctx, currentPayment.PaymentID)
							if err != nil {
								log.Printf("❌ Failed to mark email as sent for payment %s: %v", currentPayment.PaymentID, err)
							}
						}
						if telegramSent {
							err := paymentPersistence.MarkTelegramSent(ctx, currentPayment.PaymentID)
							if err != nil {
								log.Printf("❌ Failed to mark telegram as sent for payment %s: %v", currentPayment.PaymentID, err)
							}
						}
						err := paymentPersistence.UpdatePaymentCompleted(ctx, currentPayment.PaymentID)
						if err != nil {
							log.Printf("❌ Failed to mark payment completed %s: %v", currentPayment.PaymentID, err)
						} else {
							log.Printf("✅ Payment %s completed (email_sent: %v, telegram_sent: %v)", currentPayment.PaymentID, emailSent, telegramSent)
						}
					}

				case SiteTypeBalanceUpdate:
					// Cardershaven or Ganymede - update site-specific database
					log.Printf("%s payment detected - processing balance update for %s",
						siteConfig.Name, email)

					// Update site-specific database
					err = updateSiteSpecificBalance(email, balanceUSD, siteConfig.DatabaseTable)
					if err != nil {
						log.Printf("Could not update balance for %s on %s: %s",
							email, siteConfig.Name, err)
					} else {
						log.Printf("Balance updated successfully for %s on %s",
							email, siteConfig.Name)
					}

					// Send balance confirmation email
					var emailSent bool
					if userName != "User" {
						log.Println("Sending confirmation email to user:", email)
						err = utils.SendEmail(email, userName, fmt.Sprintf("%.2f", balanceUSD))
						if err != nil {
							log.Printf("Error sending email to user %s: %s", email, err)
						} else {
							log.Println("Confirmation email sent successfully to user:", email)
							emailSent = true
						}
					} else {
						log.Printf("Skipping email send for %s as user not found in database", email)
					}

					// Telegram notification for balance update
					botLogMessage := fmt.Sprintf(
						"*Email:* `%s`\n*New Balance Added (%s):* `%s USD`\n*Site:* `%s`\n*Confirmation Time:* `%s`",
						email, currencyType, fmt.Sprintf("%.2f", balanceUSD), siteConfig.Name, confirmationTime)

					msg := tgbotapi.NewMessage(chatID, botLogMessage)
					msg.ParseMode = tgbotapi.ModeMarkdown
					telegramSent := false
					_, err = bot.Send(msg)
					if err != nil {
						log.Printf("Error sending balance confirmation to bot: %s", err)
					} else {
						telegramSent = true
					}

					// Mark notifications sent in payment record
					if paymentPersistence.IsEnabled() && currentPayment != nil {
						ctx := context.Background()
						if emailSent {
							err := paymentPersistence.MarkEmailSent(ctx, currentPayment.PaymentID)
							if err != nil {
								log.Printf("❌ Failed to mark email as sent for payment %s: %v", currentPayment.PaymentID, err)
							}
						}
						if telegramSent {
							err := paymentPersistence.MarkTelegramSent(ctx, currentPayment.PaymentID)
							if err != nil {
								log.Printf("❌ Failed to mark telegram as sent for payment %s: %v", currentPayment.PaymentID, err)
							}
						}
						err := paymentPersistence.UpdatePaymentCompleted(ctx, currentPayment.PaymentID)
						if err != nil {
							log.Printf("❌ Failed to mark payment completed %s: %v", currentPayment.PaymentID, err)
						} else {
							log.Printf("✅ Payment %s completed (email_sent: %v, telegram_sent: %v)", currentPayment.PaymentID, emailSent, telegramSent)
						}
					}
				}

				return
			}

		case <-timeout:
			log.Printf("Stopped checking %s balance for address %s after %v", currencyType, address, checkDuration)
			mutex.Lock()
			delete(checkingAddresses, address)
			mutex.Unlock()
			return
		}
	}
}

// GetBitcoinAddressBalanceWithFallbackFresh - Gets balance WITHOUT using cache
// Used for critical operations like address recycling where stale data could cause issues
func GetBitcoinAddressBalanceWithFallbackFresh(address, token string) (int64, error) {
	log.Printf("🔍 FRESH balance check for %s (bypassing cache)", address)
	return getBitcoinAddressBalanceInternal(address, token, false)
}

func GetBitcoinAddressBalanceWithFallback(address, token string) (int64, error) {
	// Check cache first
	cache := payments2.GetBalanceCache()
	if cachedBalance, found := cache.Get(address); found {
		log.Printf("Using cached balance for address %s: %d satoshis", address, cachedBalance)
		return cachedBalance, nil
	}

	return getBitcoinAddressBalanceInternal(address, token, true)
}

// getBitcoinAddressBalanceInternal - Internal function that does the actual balance checking
func getBitcoinAddressBalanceInternal(address, token string, useCache bool) (int64, error) {

	circuitManager := payments2.GetCircuitBreakerManager()

	// Try providers in order with circuit breaker protection
	// Ordered by speed: fastest first for optimal detection time
	providers := []struct {
		name string
		call func() (int64, error)
	}{
		{"mempoolspace", func() (int64, error) { return payments2.GetBitcoinAddressBalanceWithMempoolSpace(address) }},      // Fastest - optimized for real-time
		{"blockstream", func() (int64, error) { return payments2.GetBitcoinAddressBalanceWithBlockStream(address) }},        // Very fast - Blockstream's infrastructure
		{"trezor", func() (int64, error) { return payments2.GetBitcoinAddressBalanceWithTrezor(address) }},                  // Fast BlockBook API
		{"blockchain", func() (int64, error) { return payments2.GetBitcoinAddressBalanceWithBlockChain(address) }},          // Original, decent speed
		{"blockcypher", func() (int64, error) { return payments2.GetBitcoinAddressBalanceWithBlockCypher(address, token) }}, // Fast but rate limited
		{"blockonomics", func() (int64, error) { return payments2.GetBitcoinAddressBalanceWithBlockonomics(address) }},      // Slower, more restricted
	}

	var lastErr error

	for _, provider := range providers {
		// Check circuit breaker
		if err := circuitManager.CanCall(provider.name); err != nil {
			log.Printf("Circuit breaker prevents call to %s: %s", provider.name, err)
			lastErr = err
			continue
		}

		// Make the API call
		balance, err := provider.call()
		if err != nil {
			log.Printf("Error with %s: %s", provider.name, err)
			circuitManager.OnFailure(provider.name)
			lastErr = err
			continue
		}

		// Success - record it, optionally cache it, and return
		circuitManager.OnSuccess(provider.name)
		if useCache {
			cache := payments2.GetBalanceCache()
			cache.Set(address, balance)
		}
		log.Printf("Balance check via %s for %s: %d satoshis (cached: %v)", provider.name, address, balance, useCache)
		return balance, nil
	}

	// All providers failed, try static address as last resort
	log.Printf("All providers failed, using static address as fallback")
	balance, err := payments2.GetBitcoinAddressBalanceWithBlockChain(staticBTCAddress)
	if err != nil {
		log.Printf("Error with static address fallback: %s", err)
		return 0, fmt.Errorf("all blockchain providers failed, last error: %v", lastErr)
	}

	return balance, nil
}

// updateSiteSpecificBalance - Helper function to update site-specific balance
func updateSiteSpecificBalance(email string, amount float64, tableName string) error {
	if tableName == "" {
		return fmt.Errorf("no database table configured for this site")
	}

	// Use parameterized query for safety
	query := fmt.Sprintf("UPDATE %s SET balance = balance + $1 WHERE email = $2", tableName)
	result, err := database.DB.Exec(query, amount, email)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user %s not found in %s", email, tableName)
	}

	return nil
}

// isStaticOrSharedAddress checks if the address is a static or shared fallback address
func isStaticOrSharedAddress(address string) bool {
	// Check if it's the main static address
	if address == staticBTCAddress || address == staticUSDTAddress {
		return true
	}

	// Check if it's one of the shared tier addresses
	for _, sharedAddr := range sharedBTCAddresses {
		if address == sharedAddr {
			return true
		}
	}

	return false
}
