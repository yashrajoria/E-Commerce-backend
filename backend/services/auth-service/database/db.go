package database

import (
	"auth-service/models"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

// ConnectPostgres creates a PostgreSQL connection with optional auto-migration for models.
// Returns a *gorm.DB or error if unsuccessful after retries.
func ConnectPostgres(autoMigrateModels ...interface{}) (*gorm.DB, error) {
	// Fetch environment variables. These should be loaded ONCE during service start (main.go).
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
	// Retry up to 10 times, with 2 seconds interval, in case DB hasn't started yet (e.g. Docker Compose/Cloud).
	for i := 0; i < 10; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			log.Println("✅ Connected to PostgreSQL successfully!")

			// Set connection pool settings for production-grade stability (optional, tweak as needed)
			sqlDB, _ := db.DB()
			sqlDB.SetMaxOpenConns(20)
			sqlDB.SetMaxIdleConns(10)
			sqlDB.SetConnMaxLifetime(2 * time.Hour)

			// Auto-migrate any provided models (e.g. models.User, models.XYZ)
			if len(autoMigrateModels) > 0 {
				if err := db.AutoMigrate(autoMigrateModels...); err != nil {
					log.Printf("❌ AutoMigrate failed: %v", err)
					return nil, fmt.Errorf("AutoMigrate failed: %w", err)
				}
			}
			return db, nil
		}
		log.Printf("❌ PostgreSQL connection failed (%d/10): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("failed to connect to PostgreSQL after retries: %w", err)
}

// Connect is your one-stop function for initializing the global DB instance.
// Just call `database.Connect()` from `main.go`.
func Connect() error {
	var err error
	DB, err = ConnectPostgres(&models.User{}, &models.RefreshToken{})
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
