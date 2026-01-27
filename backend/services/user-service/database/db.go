package database

import (
	"fmt"
	"log"
	"os"
	"time"
	"user-service/models"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func ConnectPostgres(autoMigrateModels ...interface{}) (*gorm.DB, error) {
	_ = godotenv.Load()

	dbUser := os.Getenv("POSTGRES_USER")
	dbPassword := os.Getenv("POSTGRES_PASSWORD")
	dbName := os.Getenv("POSTGRES_DB")
	dbHost := os.Getenv("POSTGRES_HOST")
	dbPort := os.Getenv("POSTGRES_PORT")
	dbSSLMode := os.Getenv("POSTGRES_SSLMODE")
	dbTimeZone := os.Getenv("POSTGRES_TIMEZONE")

	// Validate required environment variables
	if dbUser == "" {
		return nil, fmt.Errorf("POSTGRES_USER environment variable not set")
	}
	if dbPassword == "" {
		return nil, fmt.Errorf("POSTGRES_PASSWORD environment variable not set")
	}
	if dbName == "" {
		return nil, fmt.Errorf("POSTGRES_DB environment variable not set")
	}

	// Set defaults for optional variables
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

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s",
		dbHost, dbUser, dbPassword, dbName, dbPort, dbSSLMode, dbTimeZone,
	)

	var db *gorm.DB
	var err error
	for i := 0; i < 10; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			log.Println("✅ Connected to PostgreSQL successfully!")
			if len(autoMigrateModels) > 0 {
				if err := db.AutoMigrate(autoMigrateModels...); err != nil {
					return nil, fmt.Errorf("AutoMigrate failed: %w", err)
				}
			}
			return db, nil
		}
		log.Printf("❌ Connection failed (%d/10): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("failed to connect to PostgreSQL after retries: %w", err)
}

func Connect() error {
	var err error
	DB, err = ConnectPostgres(&models.User{}, &models.Address{})
	if err != nil {
		log.Println("❌ Failed to connect to PostgreSQL:", err)
		return err
	}
	return nil
}

// Close closes the database connection gracefully
func Close() error {
	if DB == nil {
		return nil
	}
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}
	return sqlDB.Close()
}
