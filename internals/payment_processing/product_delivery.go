package payment_processing

import (
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/ngenohkevin/paybutton/utils"
	"log"
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
func HandleAutomaticDelivery(email, userName, productName string, bot *tgbotapi.BotAPI) error {
	log.Printf("Handling automatic delivery for %s: %s", email, productName)

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
