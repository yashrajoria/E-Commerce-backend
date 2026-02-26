package database

import (
	"fmt"
	"os"
	"time"

	"notification-service/models"

	"github.com/joho/godotenv"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func ConnectPostgres(logger *zap.Logger, autoMigrateModels ...interface{}) (*gorm.DB, error) {
	if err := godotenv.Load(); err != nil {
		logger.Info("No .env file found, using system environment variables")
	}

	dbUser := os.Getenv("POSTGRES_USER")
	dbPassword := os.Getenv("POSTGRES_PASSWORD")
	dbName := os.Getenv("POSTGRES_DB")
	dbHost := os.Getenv("POSTGRES_HOST")
	dbPort := os.Getenv("POSTGRES_PORT")
	dbSSLMode := os.Getenv("POSTGRES_SSLMODE")
	dbTimeZone := os.Getenv("POSTGRES_TIMEZONE")

	if dbUser == "" {
		return nil, fmt.Errorf("POSTGRES_USER not set")
	}
	if dbPassword == "" {
		return nil, fmt.Errorf("POSTGRES_PASSWORD not set")
	}
	if dbName == "" {
		return nil, fmt.Errorf("POSTGRES_DB not set")
	}

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
			// Configure connection pool
			sqlDB, poolErr := db.DB()
			if poolErr == nil {
				sqlDB.SetMaxOpenConns(25)
				sqlDB.SetMaxIdleConns(5)
				sqlDB.SetConnMaxLifetime(5 * time.Minute)
			}

			logger.Info("Connected to PostgreSQL successfully")

			if len(autoMigrateModels) > 0 {
				if err := db.AutoMigrate(autoMigrateModels...); err != nil {
					return nil, fmt.Errorf("AutoMigrate failed: %w", err)
				}
			}
			return db, nil
		}

		logger.Warn("DB connection failed, retrying",
			zap.Int("attempt", i+1),
			zap.Error(err),
		)
		time.Sleep(time.Duration(i+1) * 2 * time.Second) // exponential backoff
	}

	return nil, fmt.Errorf("failed to connect to PostgreSQL after retries: %w", err)
}

func Connect(logger *zap.Logger) error {
	var err error
	DB, err = ConnectPostgres(logger, &models.NotificationLog{})
	if err != nil {
		logger.Error("Failed to connect to PostgreSQL", zap.Error(err))
		return err
	}
	return nil
}

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
