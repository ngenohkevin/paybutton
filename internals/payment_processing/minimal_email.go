package payment_processing

import (
	"fmt"
	"github.com/ngenohkevin/paybutton/utils"
	"log"
	"net/smtp"
	"strings"
)

// MinimalEmailDelivery sends a simple email notification with minimal dependencies
// This is a fallback method when other email methods fail
func MinimalEmailDelivery(to, userName, productName string) error {
	log.Printf("Attempting minimal email delivery to %s for %s", to, productName)

	config, err := utils.LoadConfig()
	if err != nil {
		log.Fatal("Error loading config:", err)
	}

	// SMTP settings
	smtpHost := config.SmtpServer
	smtpPort := 587 // Default SMTP port for ProtonMail
	smtpUsername := config.SmtpUser
	smtpPassword := config.SmtpPassword

	// Format the SMTP server address
	smtpServer := fmt.Sprintf("%s:%d", smtpHost, smtpPort)

	// Message
	subject := fmt.Sprintf("Your DWebstore Purchase: %s", productName)
	body := fmt.Sprintf("Hello %s,\n\nThank you for your purchase of %s.\n\nYour product will be delivered shortly.\n\nThank you for shopping with DWebstore!",
		userName, productName)

	// Format the email message
	message := []byte(fmt.Sprintf("To: %s\r\nSubject: %s\r\n\r\n%s", to, subject, body))

	// Authentication
	auth := smtp.PlainAuth("", smtpUsername, smtpPassword, smtpHost)

	// Send the email
	log.Printf("Connecting to %s...", smtpServer)
	err = smtp.SendMail(smtpServer, auth, smtpUsername, []string{to}, message)
	if err != nil {
		log.Printf("Failed to send minimal email: %v", err)
		return fmt.Errorf("minimal email delivery failed: %w", err)
	}

	log.Printf("Minimal email delivery successful to %s", to)
	return nil
}

// TrySendingEmail tries multiple methods to send an email
func TrySendingEmail(email, product, notification string) error {
	// Extract name if available, otherwise use "Customer"
	name := "Customer"
	if notification != "" {
		nameStart := strings.Index(notification, "**Name:** `")
		if nameStart != -1 {
			nameStart += 11 // Length of "**Name:** `"
			nameEnd := strings.Index(notification[nameStart:], "`")
			if nameEnd != -1 {
				name = notification[nameStart : nameStart+nameEnd]
			}
		}
	}

	// Try minimal approach first
	err := MinimalEmailDelivery(email, name, product)
	if err == nil {
		return nil
	}
	log.Printf("Minimal email failed: %v", err)

	// If that fails, try alternative methods
	// The implementation depends on what other email services you have access to
	// For example, you could try a third-party email API here

	return fmt.Errorf("all email delivery methods failed for %s", email)
}
