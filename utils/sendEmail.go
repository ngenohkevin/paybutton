package utils

import (
	"fmt"
	"gopkg.in/gomail.v2"
	"log"
	"os"
)

func SendEmail(userEmail string, userName string, amount string) error {
	mailPass := os.Getenv("MAILGUN_PASSWORD")
	if mailPass == "" {
		log.Fatal("MAILGUN_PASSWORD not set in .env file")
	}

	mailer := gomail.NewDialer("smtp.eu.mailgun.org", 465, "balance@cardinghaven.cc", mailPass)

	message := gomail.NewMessage()
	message.SetHeader("From", "balance@cardinghaven.cc")
	message.SetHeader("To", userEmail)
	message.SetHeader("Subject", "Payment Successful - Balance Added")
	message.SetBody("text/html", fmt.Sprintf(`
<div style="font-family: Arial, sans-serif; font-size: 16px; color: #444; background-color: #f9f9f9; padding: 20px; border: 1px solid #ddd; border-radius: 5px; max-width: 600px; margin: auto;">
    <div style="text-align: center; margin-bottom: 20px;">
        <h2 style="color: #4CAF50;">Hi %s,</h2>
    </div>
    <div style="text-align: center; margin-bottom: 20px;">
        <h1 style="color: #3B5998; font-size: 28px;">Payment Successful!</h1>
        <p style="font-size: 16px; line-height: 1.5; color: #555;">Your payment of <strong>$%s</strong> has been processed and added to your account balance.</p>
    </div>
    <div style="text-align: center; margin-bottom: 20px;">
        <p style="font-size: 16px; color: #555;">We appreciate your trust and look forward to serving you. Explore our platform to make the most of your balance.</p>
    </div>
    <div style="text-align: center; margin-bottom: 20px;">
        <p style="font-size: 16px;">
            <strong>Contact Us:</strong> <a href="mailto:cardershaven@proton.me" style="color: #007BFF; text-decoration: none;">cardershaven@proton.me</a>
        </p>
        <p style="font-size: 14px; color: #777;">Thank you for your support!</p>
    </div>
</div>
`, userName, amount))

	// Additional logging
	fmt.Println("Attempting to send email...")
	//fmt.Printf("To: %s\nSubject: %s\n", userEmail, message.GetHeader("Subject"))

	if err := mailer.DialAndSend(message); err != nil {
		fmt.Printf("Error sending email to %s: %v\n", userEmail, err)
		return fmt.Errorf("could not send email: %w", err)
	}

	fmt.Println("Email sent successfully")
	return nil
}

// SendTestEmail is a simplified function to test email sending capability
func SendTestEmail(to string) error {
	// Try sending via SendGrid (a reliable email service)
	log.Printf("Attempting to send test email to %s using SendGrid", to)

	// Fallback to simple SMTP if needed
	return sendSimpleTestEmail(to)
}

// sendSimpleTestEmail sends a test email using a simple SMTP client
func sendSimpleTestEmail(to string) error {
	// Use Gmail as a more reliable test provider
	// You would need to replace these with actual credentials
	smtpServer := "smtp.gmail.com"
	smtpPort := 587

	// For testing purposes, we could use a service like SendGrid, Mailgun, etc.
	// For now, let's use your original Mailgun settings
	smtpServer = "smtp.eu.mailgun.org"
	smtpPort = 465
	smtpUsername := "balance@cardinghaven.cc"

	// Try to get password from environment
	smtpPassword := os.Getenv("MAILGUN_PASSWORD")
	if smtpPassword == "" {
		log.Println("MAILGUN_PASSWORD not set, using fallback password")
		smtpPassword = "your-smtp-password" // Replace with actual password for testing
	}

	mailer := gomail.NewDialer(smtpServer, smtpPort, smtpUsername, smtpPassword)

	// Set SSL for Mailgun
	mailer.SSL = true

	message := gomail.NewMessage()
	message.SetHeader("From", smtpUsername)
	message.SetHeader("To", to)
	message.SetHeader("Subject", "DWebstore Test Email")
	message.SetBody("text/html", `
<div style="font-family: Arial, sans-serif; max-width: 600px; margin: auto; padding: 20px; border: 1px solid #ddd; border-radius: 5px;">
    <h1 style="color: #4CAF50; text-align: center;">Test Email</h1>
    <p style="font-size: 16px; line-height: 1.5;">This is a test email from DWebstore to verify email functionality.</p>
    <p style="font-size: 16px;">If you received this email, the email system is working correctly!</p>
</div>
`)

	log.Printf("Connecting to %s:%d...", smtpServer, smtpPort)
	if err := mailer.DialAndSend(message); err != nil {
		log.Printf("Error sending test email via %s: %v", smtpServer, err)
		return fmt.Errorf("could not send test email: %w", err)
	}

	log.Printf("Test email sent successfully to %s", to)
	return nil
}
