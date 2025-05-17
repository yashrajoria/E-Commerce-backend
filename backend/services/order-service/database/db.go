package database

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect() error {
	// Load .env file (optional)
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ No .env file found, falling back to system environment variables")
	}

	// Read environment variables
	dbUser := os.Getenv("POSTGRES_USER")
	dbPassword := os.Getenv("POSTGRES_PASSWORD")
	dbName := os.Getenv("POSTGRES_DB")
	dbHost := os.Getenv("POSTGRES_HOST")
	dbPort := os.Getenv("POSTGRES_PORT")
	dbSSLMode := os.Getenv("POSTGRES_SSLMODE")
	dbTimeZone := os.Getenv("POSTGRES_TIMEZONE")

	if dbHost == "" {
		dbHost = "localhost"
	}
	if dbPort == "" {
		dbPort = "5432"
	}
	if dbSSLMode == "" {
		dbSSLMode = "disable"
	}
	if dbTimeZone == "" {
		dbTimeZone = "Asia/Kolkata"
	}

	// Prepare DSN
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s",
		dbHost, dbUser, dbPassword, dbName, dbPort, dbSSLMode, dbTimeZone,
	)

	// Retry loop
	var err error
	for i := 0; i < 50; i++ {
		log.Printf("🔄 Attempting DB connection (%d/10)...", i+1)
		DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			log.Println("✅ Connected to PostgreSQL successfully!")
			return nil
		}
		log.Println("❌ Connection failed:", err)
		time.Sleep(2 * time.Second)
	}

	// Final failure after retries
	return fmt.Errorf("failed to connect to PostgreSQL after retries: %w", err)
}
