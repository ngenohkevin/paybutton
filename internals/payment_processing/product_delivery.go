package payment_processing

import (
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/ngenohkevin/paybutton/utils"
	"log"
	"strings"
)

// SetupBotDeliveryCommands Simplified bot handler just for manual commands
func SetupBotDeliveryCommands(bot *tgbotapi.BotAPI) {
	go func() {
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60
		updates := bot.GetUpdatesChan(u)

		for update := range updates {
			if update.Message == nil {
				continue
			}

			message := update.Message.Text

			// Check if this is a delivery command
			if strings.HasPrefix(message, "/deliver") || strings.HasPrefix(message, "!deliver") {
				// Extract the notification text from the command
				// Format: /deliver <notification>
				parts := strings.SplitN(message, " ", 2)
				if len(parts) < 2 {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "❌ Invalid format. Use: /deliver <notification>")
					bot.Send(msg)
					continue
				}

				notification := parts[1]

				// Parse the notification to extract details
				email, name, product, err := utils.ParseNotification(notification)
				if err != nil {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						fmt.Sprintf("❌ Failed to parse notification: %v", err))
					bot.Send(msg)
					continue
				}

				// Use default name if empty
				if name == "" {
					name = "Customer"
				}

				// Send the product email
				err = utils.ProductEmail(email, name, product)
				if err != nil {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						fmt.Sprintf("❌ Delivery failed: %v", err))
					bot.Send(msg)

					// Notify telegram of failure
					failMsg := utils.BuildProductPayloadForBot(email, name, product, "failed")
					failNotif := tgbotapi.NewMessage(chatID, failMsg)
					failNotif.ParseMode = tgbotapi.ModeMarkdown
					_, _ = bot.Send(failNotif)
				} else {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "✅ Product delivered successfully!")
					bot.Send(msg)

					// Notify telegram of success
					successMsg := utils.BuildProductPayloadForBot(email, name, product, "delivery")
					successNotif := tgbotapi.NewMessage(chatID, successMsg)
					successNotif.ParseMode = tgbotapi.ModeMarkdown
					_, _ = bot.Send(successNotif)
				}
			}
		}
	}()
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

//// ParsePaymentNotification parses a payment confirmation notification to extract relevant details
//func ParsePaymentNotification(notification string) (email, amount, product string, confirmationTime time.Time, err error) {
//	// Extract email
//	emailStart := strings.Index(notification, "**Email:** `")
//	if emailStart == -1 {
//		return "", "", "", time.Time{}, fmt.Errorf("email not found in notification")
//	}
//	emailStart += 12 // Length of "**Email:** `"
//	emailEnd := strings.Index(notification[emailStart:], "`")
//	if emailEnd == -1 {
//		return "", "", "", time.Time{}, fmt.Errorf("email format invalid in notification")
//	}
//	email = notification[emailStart : emailStart+emailEnd]
//
//	// Extract amount (if available)
//	amountStart := strings.Index(notification, "**New Balance Added (BTC):** `")
//	if amountStart == -1 {
//		// Try alternative format
//		amountStart = strings.Index(notification, "**Amount:** `")
//		if amountStart == -1 {
//			return email, "", "", time.Time{}, fmt.Errorf("amount not found in notification")
//		}
//		amountStart += 12 // Length of "**Amount:** `"
//	} else {
//		amountStart += 29 // Length of "**New Balance Added (BTC):** `"
//	}
//	amountEnd := strings.Index(notification[amountStart:], "`")
//	if amountEnd == -1 {
//		return email, "", "", time.Time{}, fmt.Errorf("amount format invalid in notification")
//	}
//	amount = notification[amountStart : amountStart+amountEnd]
//
//	// Extract product (if available)
//	productStart := strings.Index(notification, "**Product:** `")
//	if productStart != -1 {
//		productStart += 14 // Length of "**Product:** `"
//		productEnd := strings.Index(notification[productStart:], "`")
//		if productEnd != -1 {
//			product = notification[productStart : productStart+productEnd]
//		}
//	}
//
//	// Extract confirmation time (if available)
//	timeStart := strings.Index(notification, "**Confirmation Time:** `")
//	if timeStart != -1 {
//		timeStart += 23 // Length of "**Confirmation Time:** `"
//		timeEnd := strings.Index(notification[timeStart:], "`")
//		if timeEnd != -1 {
//			timeStr := notification[timeStart : timeStart+timeEnd]
//			confirmationTime, _ = time.Parse(time.RFC3339, timeStr)
//		}
//	} else {
//		confirmationTime = time.Now() // Use current time if confirmation time not found
//	}
//
//	return email, amount, product, confirmationTime, nil
//}

// SendProductFromNotification processes a notification message and sends the appropriate product
//func SendProductFromNotification(notification string, bot *tgbotapi.BotAPI) error {
//	log.Printf("Processing notification for product delivery: %s", notification)
//
//	// Parse the notification
//	email, _, product, _, err := ParsePaymentNotification(notification)
//	if err != nil {
//		return fmt.Errorf("failed to parse notification: %w", err)
//	}
//
//	// Find name if it exists in the notification
//	name := "Customer"
//	nameStart := strings.Index(notification, "**Name:** `")
//	if nameStart != -1 {
//		nameStart += 11 // Length of "**Name:** `"
//		nameEnd := strings.Index(notification[nameStart:], "`")
//		if nameEnd != -1 {
//			name = notification[nameStart : nameStart+nameEnd]
//		}
//	}
//
//	// If product is not found in the notification, try to find it in the session data
//	if product == "" {
//		// Get the latest product from session data
//		mutex.Lock()
//		session, exists := userSessions[email]
//		if exists && session != nil && len(session.PaymentInfo) > 0 {
//			// Get the latest payment info
//			latestPayment := session.PaymentInfo[len(session.PaymentInfo)-1]
//			product = latestPayment.Description
//		}
//		mutex.Unlock()
//	}
//
//	// If we still don't have a product, we can't proceed
//	if product == "" {
//		return fmt.Errorf("product not found in notification or session data for %s", email)
//	}
//
//	// Send the product
//	err = HandleAutomaticDelivery(email, name, product, bot)
//	if err != nil {
//		return fmt.Errorf("failed to deliver product: %w", err)
//	}
//
//	return nil
//}
