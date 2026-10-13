package db

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// PostgresDSN builds an escaped PostgreSQL URL from service configuration.
func PostgresDSN(host, port, user, password, database, sslMode, timeZone string) string {
	values := url.Values{}
	values.Set("sslmode", sslMode)
	values.Set("TimeZone", timeZone)
	query := values.Encode()
	// pgx expects the slash in an IANA timezone to remain literal in the
	// connection URL query instead of receiving the percent-encoded value.
	query = strings.Replace(query, "TimeZone="+url.QueryEscape(timeZone), "TimeZone="+timeZone, 1)

	return (&url.URL{
		Scheme:   "postgres",
		Host:     host + ":" + port,
		User:     url.UserPassword(user, password),
		Path:     "/" + database,
		RawQuery: query,
	}).String()
}

// DefaultGormConfig returns a *gorm.Config configured with a 2-second slow query
// threshold suitable for cloud PostgreSQL connections across network regions.
func DefaultGormConfig() *gorm.Config {
	return &gorm.Config{
		Logger: logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			logger.Config{
				SlowThreshold:             2 * time.Second,
				LogLevel:                  logger.Warn,
				IgnoreRecordNotFoundError: true,
				Colorful:                  false,
			},
		),
	}
}

// ConnectPostgres connects to PostgreSQL using environment variables and returns a *gorm.DB instance.
// AutoMigrate runs only when AllowAutoMigrate() is true and models are provided.
func ConnectPostgres(autoMigrateModels ...interface{}) (*gorm.DB, error) {
	_ = godotenv.Load()

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

	dsn := PostgresDSN(dbHost, dbPort, dbUser, dbPassword, dbName, dbSSLMode, dbTimeZone)

	var db *gorm.DB
	var err error
	for i := 0; i < 10; i++ {
		db, err = gorm.Open(postgres.Open(dsn), DefaultGormConfig())
		if err == nil {
			log.Println("Connected to PostgreSQL successfully!")
			if len(autoMigrateModels) > 0 && AllowAutoMigrate() {
				if err := db.AutoMigrate(autoMigrateModels...); err != nil {
					return nil, fmt.Errorf("AutoMigrate failed: %w", err)
				}
			} else if len(autoMigrateModels) > 0 {
				log.Println("⏭️  Skipping AutoMigrate (ALLOW_AUTO_MIGRATE=false); use ./scripts/migrate.sh")
			}
			return db, nil
		}
		log.Printf("Connection failed (%d/10): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("failed to connect to PostgreSQL after retries: %w", err)
}
