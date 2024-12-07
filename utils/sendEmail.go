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

	mailer := gomail.NewDialer("smtp.eu.mailgun.org", 465, "balance@cardershaven.cc", mailPass)

	message := gomail.NewMessage()
	message.SetHeader("From", "balance@cardershaven.cc")
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
        <img src="https://i.ibb.co/c6m0syN/cardshaven.png" width="120" height="120" alt="Carders Haven Logo" style="border-radius: 50%; margin-top: 10px;">
    </div>
    <div style="text-align: center; margin-bottom: 20px;">
        <p style="font-size: 16px;">
            <a href="https://t.me/stardyl" style="color: #007BFF; text-decoration: none;"><strong>Contact Us on Telegram</strong></a>
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
