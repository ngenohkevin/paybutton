package payment_processing

import (
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/ngenohkevin/paybutton/utils"
	"log"
	"strings"
)

// SetupBotDeliveryCommands Simplified bot handler using webhook instead of polling
func SetupBotDeliveryCommands(bot *tgbotapi.BotAPI) {
	// Check if we're running in production (Render) or development
	isProduction := utils.IsProduction()

	if isProduction {
		// In production, use a webhook
		webhookURL := utils.GetWebhookURL() // You'll need to implement this function

		// Remove any existing webhook
		_, _ = bot.Request(tgbotapi.DeleteWebhookConfig{})

		// Set webhook with 60-second timeout
		webhookConfig, _ := tgbotapi.NewWebhook(webhookURL)
		webhookConfig.MaxConnections = 40
		_, err := bot.Request(webhookConfig)
		if err != nil {
			log.Printf("Error setting webhook: %v", err)
		} else {
			log.Printf("Webhook set to %s", webhookURL)

			// Start webhook handler in background
			go handleWebhookUpdates(bot)
		}
	} else {
		// In development mode, only set up the bot for sending messages
		// Do not use GetUpdatesChan to avoid conflicts
		log.Printf("Bot initialized in development mode for sending only")
		// We're not setting up the updates channel to avoid conflicts
	}
}

// handleWebhookUpdates processes updates coming from the webhook
func handleWebhookUpdates(bot *tgbotapi.BotAPI) {
	// This function is now a placeholder as the actual webhook handling
	// has been moved to the server.go file with the HTTP endpoint
	log.Printf("Webhook handler started, webhook processing now handled by HTTP endpoint")
}

// HandleAutomaticDelivery sends a product email when a payment is confirmed
// Only delivers products for sites named "Dwebstore"
func HandleAutomaticDelivery(email, userName, productName, site string, bot *tgbotapi.BotAPI) error {
	log.Printf("Handling automatic delivery request for %s: %s (site: %s)", email, productName, site)

	// Check if site is authorized for automatic product delivery
	if site != "Dwebstore" && site != "dwebstore" {
		log.Printf("Automatic delivery skipped: site '%s' is not authorized for product delivery", site)
		return fmt.Errorf("site '%s' is not authorized for automatic product delivery", site)
	}

	// Send the product email
	err := utils.ProductEmail(email, userName, productName)
	if err != nil {
		log.Printf("Error in automatic product delivery: %s", err)

		// Notify the bot of failure
		if bot != nil {
			deliveryFailMsg := utils.BuildProductPayloadForBot(email, userName, productName, "failed")
			failMsg := tgbotapi.NewMessage(chatID, deliveryFailMsg)
			failMsg.ParseMode = tgbotapi.ModeMarkdown
			_, _ = bot.Send(failMsg)
		}

		return fmt.Errorf("failed to send product email: %w", err)
	}

	// Notify the bot of success
	if bot != nil {
		deliverySuccessMsg := utils.BuildProductPayloadForBot(email, userName, productName, "delivery")
		successMsg := tgbotapi.NewMessage(chatID, deliverySuccessMsg)
		successMsg.ParseMode = tgbotapi.ModeMarkdown
		_, _ = bot.Send(successMsg)
	}

	log.Printf("Automatic product delivery successful for %s", email)
	return nil
}

// HandleManualProductDelivery sends a product delivery manually for USDT payments
func HandleManualProductDelivery(email, userName, productName string, bot *tgbotapi.BotAPI, chatID int64) error {
	log.Printf("Handling manual product delivery request for %s: %s", email, productName)

	// Send the product email
	err := utils.ProductEmail(email, userName, productName)
	if err != nil {
		log.Printf("Error in manual product delivery: %s", err)

		// Notify the bot of failure
		if bot != nil {
			deliveryFailMsg := utils.BuildProductPayloadForBot(email, userName, productName, "failed")
			failMsg := tgbotapi.NewMessage(chatID, deliveryFailMsg)
			failMsg.ParseMode = tgbotapi.ModeMarkdown
			_, _ = bot.Send(failMsg)
		}

		return fmt.Errorf("failed to send product email: %w", err)
	}

	// Notify the bot of success
	if bot != nil {
		deliverySuccessMsg := utils.BuildProductPayloadForBot(email, userName, productName, "delivery")
		successMsg := tgbotapi.NewMessage(chatID, deliverySuccessMsg)
		successMsg.ParseMode = tgbotapi.ModeMarkdown
		_, _ = bot.Send(successMsg)
	}

	log.Printf("Manual product delivery successful for %s", email)
	return nil
}

// HandleManualBalanceEmailDelivery sends a balance confirmation email manually for USDT payments
func HandleManualBalanceEmailDelivery(email, userName, amountStr string, bot *tgbotapi.BotAPI, chatID int64) error {
	log.Printf("Handling manual balance email request for %s: $%s", email, amountStr)

	// Send the balance confirmation email
	err := utils.SendEmail(email, userName, amountStr)
	if err != nil {
		log.Printf("Error in manual balance email delivery: %s", err)

		// Notify the bot of failure
		if bot != nil {
			failMsg := tgbotapi.NewMessage(chatID,
				fmt.Sprintf("❌ *Balance Email Failed*\n\n*Email:* `%s`\n*Amount:* `$%s`\n*Error:* `%s`",
					email, amountStr, err.Error()))
			failMsg.ParseMode = tgbotapi.ModeMarkdown
			_, _ = bot.Send(failMsg)
		}

		return fmt.Errorf("failed to send balance email: %w", err)
	}

	// Notify the bot of success
	if bot != nil {
		successMsg := tgbotapi.NewMessage(chatID,
			fmt.Sprintf("✅ *Balance Email Sent*\n\n*Email:* `%s`\n*Amount:* `$%s`\n*Status:* `Delivered`",
				email, amountStr))
		successMsg.ParseMode = tgbotapi.ModeMarkdown
		_, _ = bot.Send(successMsg)
	}

	log.Printf("Manual balance email delivery successful for %s", email)
	return nil
}

// ParseDeliveryCommand parses a delivery command with flexible format support
// It handles both notification format and direct parameter format
func ParseDeliveryCommand(message string) (email, name, product string, err error) {
	log.Printf("Parsing delivery command: %s", message)

	// Check if message is in notification format or parameter format
	isNotificationFormat := strings.Contains(message, "**Email:**")

	if isNotificationFormat {
		// Handle notification format: **Email:** `email` **Name:** `name` **Product:** `product`
		return utils.ParseNotification(message)
	} else {
		// Handle parameter format: email name product
		parts := strings.SplitN(message, " ", 3)
		if len(parts) < 3 {
			return "", "", "", fmt.Errorf("invalid format, expected at least 3 parts: email, name, product")
		}

		email = strings.TrimSpace(parts[0])
		name = strings.TrimSpace(parts[1])
		product = strings.TrimSpace(parts[2])

		// Basic validation
		if !strings.Contains(email, "@") {
			return "", "", "", fmt.Errorf("invalid email format")
		}

		if product == "" {
			return "", "", "", fmt.Errorf("product name cannot be empty")
		}

		return email, name, product, nil
	}
}
