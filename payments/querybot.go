package payments

import (
	"database/sql"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"log"
	"os"
	"strings"
)

func QueryBot() {

	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	dbUser := os.Getenv("POSTGRES_USER")
	dbHost := os.Getenv("POSTGRES_HOST")
	dbPassword := os.Getenv("POSTGRES_PASSWORD")
	dbName := os.Getenv("POSTGRES_DATABASE")
	botToken := os.Getenv("BOT_TOKEN")

	db, err := sql.Open("postgres", fmt.Sprintf("user=%s host=%s password=%s dbname=%s sslmode=require", dbUser, dbHost, dbPassword, dbName))
	if err != nil {
		log.Fatal(err)
	}
	defer func(db *sql.DB) {
		err := db.Close()
		if err != nil {

		}
	}(db)

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatal(err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		text := update.Message.Text
		switch {
		case text == "/start":
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Hello! I am your balance updater bot. Use /update, /delete, or /show.")
			bot.Send(msg)

		case strings.HasPrefix(text, "/update"):
			// Parse the command arguments to get the email and new balance
			args := strings.Fields(text)
			if len(args) != 3 {
				reply := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /update [email] [balance]")
				bot.Send(reply)
				continue
			}

			email := args[1]
			newBalance := args[2]

			// Perform the update query
			_, err := db.Exec("UPDATE users SET balance = $1 WHERE email = $2", newBalance, email)
			if err != nil {
				reply := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Error updating balance: %v", err))
				bot.Send(reply)
				continue
			}

			reply := tgbotapi.NewMessage(update.Message.Chat.ID, "Balance updated successfully!")
			bot.Send(reply)

		case strings.HasPrefix(text, "/delete"):
			// Parse the command argument to get the email
			args := strings.Fields(text)
			if len(args) != 2 {
				reply := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /delete [email]")
				bot.Send(reply)
				continue
			}

			email := args[1]

			// Perform the delete query
			_, err := db.Exec("DELETE FROM users WHERE email = $1", email)
			if err != nil {
				reply := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Error deleting user: %v", err))
				bot.Send(reply)
				continue
			}

			reply := tgbotapi.NewMessage(update.Message.Chat.ID, "User deleted successfully!")
			bot.Send(reply)

		case strings.HasPrefix(text, "/show"):
			// Perform the query to get all users
			rows, err := db.Query("SELECT id, name, email, balance FROM users")
			if err != nil {
				reply := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Error fetching users: %v", err))
				bot.Send(reply)
				continue
			}
			defer rows.Close()

			// Prepare a response with user information
			var userString string
			for rows.Next() {
				var id, name, email string
				var balance float64
				err := rows.Scan(&id, &name, &email, &balance)
				if err != nil {
					reply := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Error scanning user: %v", err))
					bot.Send(reply)
					continue
				}
				userString += fmt.Sprintf("ID: %s\nName: %s\nEmail: %s\nBalance: %.2f\n\n", id, name, email, balance)
			}

			if userString == "" {
				userString = "No users found."
			}

			reply := tgbotapi.NewMessage(update.Message.Chat.ID, userString)
			bot.Send(reply)

		default:
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid command. Use /update, /delete, or /show.")
			bot.Send(msg)
		}
	}
}
