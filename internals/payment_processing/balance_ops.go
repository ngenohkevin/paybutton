package payment_processing

import (
	"fmt"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/ngenohkevin/paybutton/internals/database"
	payments2 "github.com/ngenohkevin/paybutton/internals/payments"
	"github.com/ngenohkevin/paybutton/utils"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
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
		// Handle USDT balance
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
				log.Printf("Address: %s, BTC Balance: %.8f BTC (%.2f USD)", address, btc, balance)
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

					// Exit the check cycle as we've detected USDT
					return
				}

				// Log different messages based on balance
				if internalBalance <= 0 {
					log.Printf("Address: %s, No USDT balance found", address)
				} else {
					log.Printf("Address: %s, USDT Balance: %.6f USDT (%.2f USD)", address, usdtBalance, internalBalance)
				}
			}

			if balance > 0 && currencyType == "BTC" {
				balanceUSD := database.RoundToTwoDecimalPlaces(balance)

				// Calculate BTC amount for WebSocket notification
				btcAmount := float64(satoshis) / 100000000 // Convert satoshis back to BTC

				// Send WebSocket notification immediately
				BroadcastBalanceUpdate(address, "confirmed", balanceUSD, btcAmount, email)

				// Try to get username from the database, but don't fail if not found
				var userName string
				err = database.DB.QueryRow("SELECT name FROM users WHERE email = $1", email).Scan(&userName)
				if err != nil {
					log.Printf("User with email %s not found in database: %s", email, err)
					userName = "User" // Set default name
				}

				// Try to update balance if possible, but don't block on failure
				err = database.UpdateUserBalance(email, balanceUSD)
				if err != nil {
					log.Printf("Could not update balance for email %s (may not exist in database): %s", email, err)
				} else {
					log.Printf("Balance updated successfully for user %s", email)
				}

				// Mark address as used
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

				confirmationTime := time.Now().Format(time.RFC3339)
				botLogMessage := fmt.Sprintf(
					"*Email:* `%s`\n*New Balance Added (%s):* `%s USD`\n*Confirmation Time:* `%s`",
					email, currencyType, fmt.Sprintf("%.2f", balanceUSD), confirmationTime)

				msg := tgbotapi.NewMessage(chatID, botLogMessage)
				msg.ParseMode = tgbotapi.ModeMarkdown
				_, err = bot.Send(msg)
				if err != nil {
					log.Printf("Error sending confirmation message to bot: %s", err)
				}

				// Extract product information from the session data
				productName := ""
				site := ""
				mutex.Lock()
				session, exists := userSessions[email]
				if exists && session != nil && len(session.PaymentInfo) > 0 {
					// Get the latest payment info
					latestPayment := session.PaymentInfo[len(session.PaymentInfo)-1]
					productName = latestPayment.Description
					site = latestPayment.Site
				}
				mutex.Unlock()

				// If we have a product name, proceed with delivery
				if productName != "" {
					log.Printf("Attempting automatic product delivery for %s: %s (site: %s)", email, productName, site)
					// Use the proper function to handle delivery
					err = HandleAutomaticDelivery(email, userName, productName, site, bot)
					if err != nil {
						log.Printf("Error in automatic product delivery: %s", err)
					} else {
						log.Printf("Automatic product delivery successful for %s", email)
					}
				} else {
					log.Printf("Skipping product delivery for %s as product not found in session", email)
				}

				// Send balance confirmation email
				if userName != "User" {
					log.Println("Sending confirmation email to user:", email)
					err = utils.SendEmail(email, userName, fmt.Sprintf("%.2f", balanceUSD))
					if err != nil {
						log.Printf("Error sending email to user %s: %s", email, err)
					} else {
						log.Println("Confirmation email sent successfully to user:", email)
					}
				} else {
					log.Printf("Skipping email send for %s as user not found in database", email)
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

func GetBitcoinAddressBalanceWithFallback(address, token string) (int64, error) {
	balance, err := payments2.GetBitcoinAddressBalanceWithBlockChain(address)
	if err != nil {
		log.Printf("Error with Blockchain, trying Blockonomics: %s", err)
		balance, err = payments2.GetBitcoinAddressBalanceWithBlockonomics(address)
		if err != nil {
			log.Printf("Error with Blockonomics, trying BlockCypher: %s", err)
			balance, err = payments2.GetBitcoinAddressBalanceWithBlockCypher(address, token)
		}
		if err != nil {
			log.Printf("Error with BlockCypher, using static address: %s", err)
			balance, err = payments2.GetBitcoinAddressBalanceWithBlockChain(staticBTCAddress)
		}
	}
	return balance, err
}
