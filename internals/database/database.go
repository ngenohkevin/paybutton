package database

import (
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"github.com/ngenohkevin/paybutton/utils"
	"log"
	"math"
	"time"
)

var (
	DB *sql.DB
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

	log.Printf("Attempting to connect to PostgreSQL database: %s@%s/%s",
		PostgresUser, PostgresHost, PostgresDatabase)

	connStr := fmt.Sprintf(
		"user=%s host=%s password=%s dbname=%s sslmode=disable connect_timeout=10",
		PostgresUser, PostgresHost, PostgresPassword, PostgresDatabase)

	var dbErr error

	DB, dbErr = sql.Open("postgres", connStr)
	if dbErr != nil {
		return fmt.Errorf("error opening database connection: %w", dbErr)
	}

	// Configure connection pool
	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(5)
	DB.SetConnMaxLifetime(5 * time.Minute)
	DB.SetConnMaxIdleTime(10 * time.Minute)

	pingErr := DB.Ping()
	if pingErr != nil {
		return fmt.Errorf("database is unreachable with both SSL modes: %w", pingErr)
	}

	log.Println("Successfully connected to the database")
	return nil
}

// CloseDB closes the database connection
func CloseDB() {
	if DB != nil {
		_ = DB.Close()
		log.Println("Successfully closed the database")
	}
}

func UpdateUserBalance(email string, newBalanceUSD float64) error {
	var currentBalance float64

	err := DB.QueryRow("SELECT balance FROM users WHERE email = $1", email).Scan(&currentBalance)
	if err != nil {
		return fmt.Errorf("error fetching current balance for user %s: %w", email, err)
	}

	updatedBalance := RoundToTwoDecimalPlaces(currentBalance + newBalanceUSD)

	_, err = DB.Exec("UPDATE users SET balance = $1 WHERE email = $2", updatedBalance, email)
	if err != nil {
		return fmt.Errorf("error updating balance for user %s: %w", email, err)
	}

	return nil
}

func RoundToTwoDecimalPlaces(value float64) float64 {
	return math.Round(value*100) / 100
}
