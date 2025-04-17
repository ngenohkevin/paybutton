package utils

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"gopkg.in/gomail.v2"
	"io"
	"log"
	"math/big"
	"strings"
	"time"
)

// ProductEmail sends an email with the purchased product as an attachment
func ProductEmail(userEmail, userName, productName string) error {
	log.Printf("Starting product email delivery for: %s to %s", productName, userEmail)

	config, err := LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	//smtpPort := protonSmtpPort
	smtpUsername := config.SmtpUser
	smtpPassword := config.SmtpPassword
	smtpServer := config.SmtpServer
	smtpPort := 587

	mailer := gomail.NewDialer(smtpServer, smtpPort, smtpUsername, smtpPassword)

	// Configure for ProtonMail - use TLS
	mailer.SSL = false // We'll use STARTTLS instead of direct SSL
	mailer.TLSConfig = &tls.Config{
		ServerName:         smtpServer,
		InsecureSkipVerify: false,
	}

	message := gomail.NewMessage()
	message.SetHeader("From", smtpUsername)
	message.SetHeader("To", userEmail)
	message.SetHeader("Subject", "Your Dwebstore Purchase - Product Delivery")

	// Check if the product is the RPSX decryption tool
	isRPSXTool := CheckIfRPSXTool(productName)

	if isRPSXTool {
		// Special email for RPSX tool with download link
		rpsxToolLink := "https://mega.nz/file/TyQ0hC4b#P7fsFAEIWFrrpBrhcQt6ElavppkuQr5CWo5xEDmCUEw"
		message.SetBody("text/html", fmt.Sprintf(`
<div style="font-family: Arial, sans-serif; font-size: 16px; color: #444; background-color: #f9f9f9; padding: 20px; border: 1px solid #ddd; border-radius: 5px; max-width: 600px; margin: auto;">
    <div style="text-align: center; margin-bottom: 20px;">
        <h2 style="color: #4CAF50;">Hello!</h2>
    </div>
    <div style="text-align: center; margin-bottom: 20px;">
        <h1 style="color: #3B5998; font-size: 28px;">Your RPSX Decryption Tool</h1>
        <p style="font-size: 16px; line-height: 1.5; color: #555;">Thank you for your purchase of the <strong>RPSX Decryption Tool</strong>.</p>
    </div>
    <div style="text-align: center; margin-bottom: 20px;">
        <p style="font-size: 16px; color: #555;">You can download the RPSX Decryption Tool from the following link:</p>
        <p style="margin: 20px 0;"><a href="%s" style="background-color: #4CAF50; color: white; padding: 10px 20px; text-decoration: none; border-radius: 5px; font-weight: bold;">Download RPSX Decryption Tool</a></p>
        <p style="font-size: 14px; color: #777;"><em>Note: Please open this tool with Windows for best compatibility.</em></p>
    </div>
    <div style="margin-bottom: 20px; padding: 15px; background-color: #f0f0f0; border-left: 4px solid #4CAF50; border-radius: 3px;">
        <h3 style="margin-top: 0; color: #333;">Instructions:</h3>
        <ol style="text-align: left; color: #555;">
            <li>Download the tool using the button above</li>
            <li>Extract the files if necessary</li>
            <li>Run the application</li>
            <li>Use the tool to decrypt any .rpsx files you purchase from us</li>
        </ol>
    </div>
    <div style="text-align: center; margin-bottom: 20px;">
        <p style="font-size: 14px; color: #777;"><em>
			Note: This tool might be flagged as malicious by antivirus. 
			But it is a false positive and it has nothing malicious in it. 
			You can run on a sandbox or a vm if you're skeptical.
		</em></p>
        <p style="font-size: 16px; color: #555;">If you have any questions or need assistance with the tool, please don't hesitate to contact our support team.</p>
    </div>
    <div style="text-align: center; margin-top: 30px; border-top: 1px solid #ddd; padding-top: 20px;">
        <p style="font-size: 14px; color: #777;">Thank you for shopping with DWebstore!</p>
    </div>
</div>
`, rpsxToolLink))
	} else {
		// Generate product file content (1-4MB)
		fileContent, err := GenerateRandomBytes(1*1024*1024, 4*1024*1024)
		if err != nil {
			return fmt.Errorf("error generating product file content: %w", err)
		}

		// Generate appropriate filename
		filename := GenerateProductFilename(productName)

		// Attach the file using gomail's methods
		message.Attach(
			filename,
			gomail.SetCopyFunc(func(w io.Writer) error {
				_, err := w.Write(fileContent)
				return err
			}),
		)

		// Email body for products
		message.SetBody("text/html", fmt.Sprintf(`
<div style="font-family: Arial, sans-serif; font-size: 16px; color: #444; background-color: #f9f9f9; padding: 20px; border: 1px solid #ddd; border-radius: 5px; max-width: 600px; margin: auto;">
    <div style="text-align: center; margin-bottom: 20px;">
        <h2 style="color: #4CAF50;">Hello!</h2>
    </div>
    <div style="text-align: center; margin-bottom: 20px;">
        <h1 style="color: #3B5998; font-size: 28px;">Your Purchase is Ready</h1>
        <p style="font-size: 16px; line-height: 1.5; color: #555;">Thank you for your purchase of <strong>%s</strong>.</p>
        <p style="font-size: 16px; line-height: 1.5; color: #555;">Your product file has been attached to this email.</p>
    </div>
    <div style="margin-bottom: 20px; padding: 15px; background-color: #f0f0f0; border-left: 4px solid #4CAF50; border-radius: 3px;">
        <h3 style="margin-top: 0; color: #333;">Important Information:</h3>
        <ul style="text-align: left; color: #555;">
            <li>To open the file, you'll need the RPSX Decryption Tool</li>
            <li>it's available in our store under Tools & Tutorials</li>
        </ul>
    </div>
    <div style="text-align: center; margin-bottom: 20px;">
        <p style="font-size: 16px; color: #555;">If you have any questions about your purchase or need assistance, please don't hesitate to contact our support team.</p>
    </div>
    <div style="text-align: center; margin-top: 30px; border-top: 1px solid #ddd; padding-top: 20px;">
        <p style="font-size: 14px; color: #777;">Thank you for shopping with DWebstore!</p>
    </div>
</div>
`, productName))
	}

	// Send the email
	log.Printf("Attempting to send product delivery email to %s for product %s", userEmail, productName)
	if err := mailer.DialAndSend(message); err != nil {
		log.Printf("Error sending product delivery email to %s: %v", userEmail, err)
		return fmt.Errorf("could not send product email: %w", err)
	}

	log.Printf("Product delivery email sent successfully to %s", userEmail)
	return nil
}

// ParseNotification parses the notification message to extract details for delivery
func ParseNotification(notificationText string) (email, name, product string, err error) {
	log.Printf("Parsing notification: %s", notificationText)

	// Extract email using string operations to avoid regex issues
	emailStart := strings.Index(notificationText, "**Email:** `")
	if emailStart == -1 {
		return "", "", "", fmt.Errorf("email not found in notification")
	}
	emailStart += 12 // Length of "**Email:** `"
	emailEnd := strings.Index(notificationText[emailStart:], "`")
	if emailEnd == -1 {
		return "", "", "", fmt.Errorf("email format invalid in notification")
	}
	email = notificationText[emailStart : emailStart+emailEnd]

	// Extract name using string operations
	nameStart := strings.Index(notificationText, "**Name:** `")
	if nameStart == -1 {
		log.Printf("Name not found in notification, using 'Customer' as default")
		return email, "Customer", "", nil
	}
	nameStart += 11 // Length of "**Name:** `"
	nameEnd := strings.Index(notificationText[nameStart:], "`")
	if nameEnd == -1 {
		return email, "Customer", "", fmt.Errorf("name format invalid in notification")
	}
	name = notificationText[nameStart : nameStart+nameEnd]

	// Extract product using string operations
	productStart := strings.Index(notificationText, "**Product:** `")
	if productStart == -1 {
		return email, name, "", fmt.Errorf("product not found in notification")
	}
	productStart += 14 // Length of "**Product:** `"
	productEnd := strings.Index(notificationText[productStart:], "`")
	if productEnd == -1 {
		return email, name, "", fmt.Errorf("product format invalid in notification")
	}
	product = notificationText[productStart : productStart+productEnd]

	log.Printf("Parsed notification - Email: %s, Name: %s, Product: %s", email, name, product)
	return email, name, product, nil
}

// BuildProductPayloadForBot formats the notification for the Telegram bot
func BuildProductPayloadForBot(email, name, product, notificationType string) string {
	var message bytes.Buffer
	currentTime := time.Now().Format("2006-01-02 15:04:05")

	if notificationType == "delivery" {
		message.WriteString("✅ *Product Delivered*\n\n")
		message.WriteString(fmt.Sprintf("*Email:* `%s`\n", email))
		message.WriteString(fmt.Sprintf("*Product:* `%s`\n", product))
		message.WriteString(fmt.Sprintf("*Status:* `Delivered`\n"))
		message.WriteString(fmt.Sprintf("*Time:* `%s`\n", currentTime))
	} else {
		message.WriteString("⚠️ *Delivery Failed*\n\n")
		message.WriteString(fmt.Sprintf("*Email:* `%s`\n", email))
		message.WriteString(fmt.Sprintf("*Product:* `%s`\n", product))
		message.WriteString(fmt.Sprintf("*Status:* `Failed`\n"))
		message.WriteString(fmt.Sprintf("*Time:* `%s`\n", currentTime))
	}

	return message.String()
}

// CheckIfRPSXTool checks if the product is the RPSX decryption tool
func CheckIfRPSXTool(productName string) bool {
	productNameLower := strings.ToLower(productName)
	return strings.Contains(productNameLower, "rpsx decryption tool") ||
		strings.Contains(productNameLower, "rpsx tool") ||
		strings.Contains(productNameLower, "decryption tool")
}

// GenerateRandomBytes generates a random byte slice of size between min and max
func GenerateRandomBytes(min, max int64) ([]byte, error) {
	// Generate random size between min and max
	diff := max - min
	n, err := rand.Int(rand.Reader, big.NewInt(diff))
	if err != nil {
		return nil, err
	}
	size := n.Int64() + min

	// Create buffer and fill with random data
	buf := make([]byte, size)
	_, err = rand.Read(buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

// GenerateProductFilename creates the appropriate filename based on product name
func GenerateProductFilename(productName string) string {
	log.Printf("Generating filename for product: %s", productName)

	// Check if this is the RPSX tool (should not happen here, but just in case)
	if CheckIfRPSXTool(productName) {
		return "RPSX_Decryption_Tool.txt"
	}

	// Check special cases for Visa and Mastercard
	productLower := strings.ToLower(productName)
	if strings.Contains(productLower, "visa") {
		return "visa.rpsx"
	}
	if strings.Contains(productLower, "mastercard") {
		return "mastercard.rpsx"
	}

	// Look for dollar amount using a simple string search
	dollarIndex := strings.Index(productName, "$")
	if dollarIndex != -1 {
		// Find the end of the dollar amount (first space after $ or end of string)
		amountEnd := strings.Index(productName[dollarIndex:], " ")
		if amountEnd == -1 {
			// No space, use the rest of the string
			amountEnd = len(productName) - dollarIndex
		} else {
			amountEnd += dollarIndex
		}

		// Extract the amount and the product type
		amount := productName[dollarIndex:amountEnd]
		productType := ""
		if amountEnd < len(productName) {
			productType = strings.TrimSpace(productName[amountEnd:])
		}

		// Remove spaces and format the filename
		productType = strings.Replace(productType, " ", "", -1)

		filename := fmt.Sprintf("%s%sLog.rpsx", amount, productType)
		log.Printf("Generated filename with dollar amount: %s", filename)
		return filename
	}

	// Handle other products
	// Default case: just use the product name without spaces
	cleanName := strings.Replace(productName, " ", "", -1)
	filename := fmt.Sprintf("%sLog.rpsx", cleanName)
	log.Printf("Generated generic filename: %s", filename)
	return filename
}
