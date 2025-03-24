package database

import (
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"github.com/ngenohkevin/paybutton/utils"
	"log"
	"math"
)

var (
	db *sql.DB
)

func InitDB() error {

	config, err := utils.LoadConfig()
	if err != nil {
		log.Fatal("Error loading config:", err)
	}

	//load the config values
	PostgresUser := config.PostgresUser
	PostgresHost := config.PostgresHost
	PostgresPassword := config.PostgresPassword
	PostgresDatabase := config.PostgresDB

	db, err = sql.Open("postgres", fmt.Sprintf("user=%s host=%s password=%s dbname=%s sslmode=require", PostgresUser, PostgresHost, PostgresPassword, PostgresDatabase))
	if err != nil {
		log.Fatal("Error connecting to the database:", err)
	}
	defer func(db *sql.DB) {
		err := db.Close()
		if err != nil {
			return
		}
	}(db)

	// Check if the database connection is alive
	err = db.Ping()
	if err != nil {
		return fmt.Errorf("database is unreachable: %w", err)
	}

	log.Println("Successfully connected to the database")

	return nil
}

// CloseDB closes the database connection
func CloseDB() {
	if db != nil {
		_ = db.Close()
		log.Println("Successfully closed the database")
	}
}

func UpdateUserBalance(email string, newBalanceUSD float64) error {
	var currentBalance float64

	err := db.QueryRow("SELECT balance FROM users WHERE email = $1", email).Scan(&currentBalance)
	if err != nil {
		return fmt.Errorf("error fetching current balance for user %s: %w", email, err)
	}

	updatedBalance := RoundToTwoDecimalPlaces(currentBalance + newBalanceUSD)

	_, err = db.Exec("UPDATE users SET balance = $1 WHERE email = $2", updatedBalance, email)
	if err != nil {
		return fmt.Errorf("error updating balance for user %s: %w", email, err)
	}

	return nil
}

func RoundToTwoDecimalPlaces(value float64) float64 {
	return math.Round(value*100) / 100
}
