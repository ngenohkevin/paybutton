package payments

import (
	"database/sql"
	"errors"
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
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Hello! I am your balance updater bot. Use /update, /delete, /show or /show_user.")
			_, err := bot.Send(msg)
			if err != nil {
				return
			}

		case strings.HasPrefix(text, "/update"):
			// Parse the command arguments to get the email and new balance
			args := strings.Fields(text)
			if len(args) != 3 {
				reply := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /update [email] [balance]")
				_, err := bot.Send(reply)
				if err != nil {
					return
				}
				continue
			}

			email := args[1]
			newBalance := args[2]

			// Perform the update query
			_, err := db.Exec("UPDATE users SET balance = $1 WHERE email = $2", newBalance, email)
			if err != nil {
				reply := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Error updating balance: %v", err))
				_, err := bot.Send(reply)
				if err != nil {
					return
				}
				continue
			}

			reply := tgbotapi.NewMessage(update.Message.Chat.ID, "Balance updated successfully!")
			_, err = bot.Send(reply)
			if err != nil {
				return
			}

		case strings.HasPrefix(text, "/delete"):
			// Parse the command argument to get the email
			args := strings.Fields(text)
			if len(args) != 2 {
				reply := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /delete [email]")
				_, err := bot.Send(reply)
				if err != nil {
					return
				}
				continue
			}

			email := args[1]

			// Perform the delete query
			_, err := db.Exec("DELETE FROM users WHERE email = $1", email)
			if err != nil {
				reply := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Error deleting user: %v", err))
				_, err := bot.Send(reply)
				if err != nil {
					return
				}
				continue
			}

			reply := tgbotapi.NewMessage(update.Message.Chat.ID, "User deleted successfully!")
			_, err = bot.Send(reply)
			if err != nil {
				return
			}

		case strings.HasPrefix(text, "/show"):
			// Perform the query to get all users
			rows, err := db.Query("SELECT id, name, email, balance, password FROM users")
			if err != nil {
				reply := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Error fetching users: %v", err))
				_, err := bot.Send(reply)
				if err != nil {
					return
				}
				continue
			}
			defer func(rows *sql.Rows) {
				err := rows.Close()
				if err != nil {

				}
			}(rows)

			// Prepare a response with user information
			var userString string
			for rows.Next() {
				var id, name, email, password string
				var balance float64
				err := rows.Scan(&id, &name, &email, &balance, &password)
				if err != nil {
					reply := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Error scanning user: %v", err))
					_, err := bot.Send(reply)
					if err != nil {
						return
					}
					continue
				}
				userString += fmt.Sprintf("ID: %s\nName: %s\nEmail: %s\nBalance: %.2f\nPassword: %s\n\n", id, name, email, balance, password)
			}

			if userString == "" {
				userString = "No users found."
			}

			reply := tgbotapi.NewMessage(update.Message.Chat.ID, userString)
			_, err = bot.Send(reply)
			if err != nil {
				return
			}

		case strings.HasPrefix(text, "/show_user"):
			args := strings.Fields(text)
			if len(args) != 2 {
				reply := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /show_user [email]")
				_, err := bot.Send(reply)
				if err != nil {
					return
				}
				continue
			}

			email := args[1]

			// Perform the query to get the user with the specified email
			row := db.QueryRow("SELECT id, name, email, balance, password FROM users WHERE email = $1", email)

			// Check if the user exists
			var id, name, password string
			var balance float64
			switch err := row.Scan(&id, &name, &email, &balance, &password); {
			case errors.Is(err, sql.ErrNoRows):
				reply := tgbotapi.NewMessage(update.Message.Chat.ID, "User not found.")
				_, err := bot.Send(reply)
				if err != nil {
					return
				}
				continue
			case err == nil:
				userString := fmt.Sprintf("ID: %s\nName: %s\nEmail: %s\nBalance: %.2f\nPassword: %s", id, name, email, balance, password)
				reply := tgbotapi.NewMessage(update.Message.Chat.ID, userString)
				_, err := bot.Send(reply)
				if err != nil {
					return
				}
			default:
				reply := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Error fetching user: %v", err))
				_, err := bot.Send(reply)
				if err != nil {
					return
				}
				continue
			}
		}
	}
}
