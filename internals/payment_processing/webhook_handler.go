package payment_processing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/ngenohkevin/paybutton/internals/database"
	"github.com/ngenohkevin/paybutton/utils"
)

// BlockonomicsWebhookPayload represents the webhook payload from Blockonomics
type BlockonomicsWebhookPayload struct {
	Status        int    `json:"status"`                  // 0=unconfirmed, 1=partially confirmed, 2=confirmed
	Value         int64  `json:"value"`                   // Value in satoshis
	TxID          string `json:"txid"`                    // Transaction ID
	Address       string `json:"addr"`                    // Bitcoin address
	Confirmations int    `json:"confirmations,omitempty"` // Number of confirmations
}

// WebhookSecret for verifying webhook authenticity (should be set via environment)
var webhookSecret = "your-webhook-secret-here" // TODO: Move to config

// HandleBlockonomicsWebhook handles incoming webhooks from Blockonomics
func HandleBlockonomicsWebhook(c *gin.Context, bot *tgbotapi.BotAPI) {
	// Read the raw body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("Error reading webhook body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Verify webhook signature (if using secret)
	if webhookSecret != "your-webhook-secret-here" {
		signature := c.GetHeader("X-Webhook-Signature")
		if !verifyWebhookSignature(body, signature, webhookSecret) {
			log.Printf("Invalid webhook signature")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid signature"})
			return
		}
	}

	// Parse the webhook payload
	var payload BlockonomicsWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("Error parsing webhook payload: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON payload"})
		return
	}

	log.Printf("🚀 WEBHOOK RECEIVED: Address %s, Value: %d satoshis, Status: %d, TxID: %s",
		payload.Address, payload.Value, payload.Status, payload.TxID)

	// Only process confirmed transactions (status 2) or partially confirmed (status 1)
	if payload.Status < 1 {
		log.Printf("Ignoring unconfirmed transaction for address %s", payload.Address)
		c.JSON(http.StatusOK, gin.H{"message": "Transaction not confirmed yet"})
		return
	}

	// Find the email associated with this address
	email := findEmailForAddress(payload.Address)
	if email == "" {
		log.Printf("No email found for address %s, webhook ignored", payload.Address)
		c.JSON(http.StatusOK, gin.H{"message": "Address not tracked"})
		return
	}

	// Convert satoshis to BTC and USD
	btcAmount := float64(payload.Value) / 100000000
	rate, err := utils.GetBlockonomicsRate()
	if err != nil {
		log.Printf("Error fetching BTC rate for webhook: %s", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Rate fetch failed"})
		return
	}
	balanceUSD := btcAmount * rate
	balanceUSDRounded := database.RoundToTwoDecimalPlaces(balanceUSD)

	// Send instant WebSocket and SSE notification
	BroadcastBalanceUpdateAll(payload.Address, "confirmed", balanceUSDRounded, btcAmount, email)

	// Update session status to completed via callback if available
	if SessionStatusUpdater != nil {
		SessionStatusUpdater(payload.Address, "completed")
	}

	// Send Telegram notification
	confirmationTime := getCurrentTimestamp()
	botLogMessage := fmt.Sprintf(
		"⚡ *INSTANT WEBHOOK NOTIFICATION*\n*Email:* `%s`\n*Address:* `%s`\n*Amount:* `%.8f BTC ($%.2f)`\n*TxID:* `%s`\n*Confirmations:* `%d`\n*Time:* `%s`",
		email, payload.Address, btcAmount, balanceUSDRounded, payload.TxID, payload.Confirmations, confirmationTime)

	msg := tgbotapi.NewMessage(chatID, botLogMessage)
	msg.ParseMode = tgbotapi.ModeMarkdown
	_, err = bot.Send(msg)
	if err != nil {
		log.Printf("Error sending webhook notification to bot: %s", err)
	}

	// Try to get username from database (but don't fail if not found)
	var userName string
	err = database.DB.QueryRow("SELECT name FROM users WHERE email = $1", email).Scan(&userName)
	if err != nil {
		log.Printf("User with email %s not found in database: %s", email, err)
		userName = "User" // Set default name
	}

	// Extract product information first
	productName := ""
	site := ""
	mutex.Lock()
	session := userSessions[email]
	if session != nil && len(session.PaymentInfo) > 0 {
		latestPayment := session.PaymentInfo[len(session.PaymentInfo)-1]
		productName = latestPayment.Description
		site = latestPayment.Site
	}
	mutex.Unlock()

	// Mark address as used and clean up tracking
	mutex.Lock()
	if session != nil {
		session.UsedAddresses[payload.Address] = true
		if len(session.UsedAddresses) > 0 && !session.ExtendedAddressAllowed {
			session.ExtendedAddressAllowed = true
		}
	}
	delete(checkingAddresses, payload.Address) // Stop polling since webhook received
	mutex.Unlock()

	// Site-based conditional logic (same as balance_ops.go)
	if site == "Dwebstore" || site == "dwebstore" {
		// DWEBSTORE: Product delivery flow
		log.Printf("🚀 Webhook: Dwebstore payment detected - processing product delivery for %s: %s", email, productName)

		if productName != "" {
			err = HandleAutomaticDelivery(email, userName, productName, site, bot)
			if err != nil {
				log.Printf("Error in webhook product delivery: %s", err)
			} else {
				log.Printf("✅ Instant webhook product delivery successful for %s", email)
			}
		} else {
			log.Printf("Skipping webhook product delivery for %s as product not found in session", email)
		}

	} else {
		// CARDERSHAVEN or other sites: Balance update flow
		log.Printf("🚀 Webhook: Cardershaven/other payment detected - processing balance update for %s", email)

		// Update database balance
		err = database.UpdateUserBalance(email, balanceUSDRounded)
		if err != nil {
			log.Printf("Could not update balance for email %s: %s", email, err)
		} else {
			log.Printf("✅ Balance updated via webhook for user %s", email)
		}

		// Send balance confirmation email
		if userName != "User" {
			log.Println("📧 Sending webhook confirmation email to user:", email)
			err = utils.SendEmail(email, userName, fmt.Sprintf("%.2f", balanceUSDRounded))
			if err != nil {
				log.Printf("Error sending webhook email to user %s: %s", email, err)
			} else {
				log.Println("✅ Webhook confirmation email sent successfully to user:", email)
			}
		} else {
			log.Printf("Skipping webhook email send for %s as user not found in database", email)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Webhook processed successfully",
		"address": payload.Address,
		"amount":  fmt.Sprintf("%.8f BTC", btcAmount),
		"usd":     fmt.Sprintf("$%.2f", balanceUSDRounded),
	})

	log.Printf("🎉 WEBHOOK PROCESSING COMPLETE for %s - %.8f BTC ($%.2f)", payload.Address, btcAmount, balanceUSDRounded)
}

// findEmailForAddress finds the email associated with a Bitcoin address
func findEmailForAddress(address string) string {
	mutex.Lock()
	defer mutex.Unlock()

	// Search through all user sessions to find the address
	for email, session := range userSessions {
		if session != nil {
			// Check if this address exists in generated addresses
			if _, exists := session.GeneratedAddresses[address]; exists {
				return email
			}
		}
	}
	return ""
}

// verifyWebhookSignature verifies the webhook signature using HMAC-SHA256
func verifyWebhookSignature(body []byte, signature string, secret string) bool {
	if signature == "" {
		return false
	}

	// Remove "sha256=" prefix if present
	signature = strings.TrimPrefix(signature, "sha256=")

	// Calculate expected signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	// Compare signatures
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// RegisterWebhookWithBlockonomics registers a webhook URL with Blockonomics (optional helper)
func RegisterWebhookWithBlockonomics(webhookURL string) error {
	// This would make an API call to Blockonomics to register the webhook
	// Implementation depends on Blockonomics API
	log.Printf("TODO: Register webhook URL with Blockonomics: %s", webhookURL)
	return nil
}
