package utils

import (
	"github.com/joho/godotenv"
	"os"
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
	err = godotenv.Load(".env")
	if err != nil {
		return Config{}, err
	}

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
	smtpServer := os.Getenv("PROTON_SMTP_SERVER")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPassword := os.Getenv("SMTP_PASSWORD")

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

	return config, err
}
