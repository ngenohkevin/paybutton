package utils

import (
	"fmt"
	"gopkg.in/gomail.v2"
	"log"
	"os"
)

func SendEmail(userEmail string, userName string, amount string) error {
	mailPass := os.Getenv("ZOHO_MAIL_PASSWORD")
	if mailPass == "" {
		log.Fatal("ZOHO_MAIL_PASSWORD not set in .env file")
	}

	mailer := gomail.NewDialer("smtp.zoho.com", 465, "balanceupdate@cardershaven.cc", mailPass)

	message := gomail.NewMessage()
	message.SetHeader("From", "balanceupdate@cardershaven.cc")
	message.SetHeader("To", userEmail)
	message.SetHeader("Subject", "Payment Successful - Balance Added")
	message.SetBody("text/html", fmt.Sprintf(`
<div style="font-family: Arial, Helvetica, sans-serif; font-size: 14px; color: #333; padding: 20px;">
    <div style="text-align: center; margin-bottom: 20px;">
        <h2 style="color: #4CAF50;">Hello %s,</h2>
    </div>
    <div style="text-align: center; margin-bottom: 20px;">
        <h1 style="color: #3B5998; font-size: 32px;">Payment Successful</h1>
    </div>
    <div style="text-align: center; margin-bottom: 20px;">
        <p style="font-size: 18px;">Your payment of $%s has been successfully processed and added to your account balance.</p>
        <p style="font-size: 18px;">You can now use your balance to purchase products from our platform.</p>
    </div>
    <div style="text-align: center; margin-bottom: 20px;">
        <p style="font-size: 18px;">Sometimes we experience a purchase frenzy and we can run out of stock. But we always stock up</p>
        <p style="font-size: 18px; color: #4CAF50;">Add a higher balance to avoid out of stock due to frenzy purchase</p>
    </div>
    <div style="text-align: center; margin-bottom: 20px;">
        <img src="https://i.ibb.co/c6m0syN/cardshaven.png" width="150" height="150" alt="Carders Haven Logo" style="border-radius: 50%;">
    </div>
    <div style="text-align: center; margin-bottom: 20px;">
        <p style="font-size: 18px;">
            <a href="mailto:michaels@cardershaven.cc" style="color: #007BFF; text-decoration: none;"><b>Email me</b></a>
            &nbsp;or&nbsp;
            <a href="https://t.me/stardyl" style="color: #007BFF; text-decoration: none;"><b>Contact me on Telegram</b></a>
        </p>
        <p style="font-size: 18px;">Start Shopping!! and don't forget to send vouches to me on telegram</p>
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
