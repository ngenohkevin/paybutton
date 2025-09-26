package utils

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	PostgresHost     string
	BotApiKey        string
	BlockCypherToken string
	BlockonomicsAPI  string
	IpLocation       string
	MailGunAPIKey    string
	MailGunPassword  string
	TronScanAPI      string
	SmtpServer       string
	SmtpUser         string
	SmtpPassword     string
}

func LoadConfig() (config Config, err error) {
	// Try to load .env file, but don't fail if it doesn't exist (for container deployments)
	err = godotenv.Load(".env")
	if err != nil {
		log.Printf("Warning: .env file not found, using environment variables directly")
	} else {
		log.Printf("Loaded configuration from .env file")
	}

	// Load environment variables
	user := os.Getenv("POSTGRES_USER")
	host := os.Getenv("POSTGRES_HOST")
	password := os.Getenv("POSTGRES_PASSWORD")
	db := os.Getenv("POSTGRES_DATABASE")
	bot := os.Getenv("BOT_API_KEY")
	blockCypherToken := os.Getenv("BLOCKCYPHER_TOKEN")
	blockonomicsAPIKey := os.Getenv("BLOCKONOMICS_API_KEY")
	ipLocation := os.Getenv("IP_LOCATION_API_KEY")
	mailGunAPI := os.Getenv("MAILGUN_API_KEY")
	mailGunPass := os.Getenv("MAILGUN_PASSWORD")
	tronScanAPI := os.Getenv("TRONSCAN_API")
	smtpServer := "smtp.zoho.com"
	smtpUser := os.Getenv("ZOHO_MAIL")
	smtpPassword := os.Getenv("ZOHO_MAIL_PASSWORD")

	// Log critical environment variables (without sensitive data)
	log.Printf("Config loaded - Database: %s@%s/%s", user, host, db)
	log.Printf("Bot API Key present: %v", bot != "")
	log.Printf("SMTP configured: %s with user %s", smtpServer, smtpUser)
	log.Printf("Mailgun configured: %v", mailGunPass != "")
	log.Printf("Blockonomics API configured: %v", blockonomicsAPIKey != "")

	config = Config{
		PostgresUser:     user,
		PostgresPassword: password,
		PostgresDB:       db,
		PostgresHost:     host,
		BotApiKey:        bot,
		BlockCypherToken: blockCypherToken,
		BlockonomicsAPI:  blockonomicsAPIKey,
		IpLocation:       ipLocation,
		MailGunAPIKey:    mailGunAPI,
		MailGunPassword:  mailGunPass,
		TronScanAPI:      tronScanAPI,
		SmtpServer:       smtpServer,
		SmtpUser:         smtpUser,
		SmtpPassword:     smtpPassword,
	}

	return config, nil
}
