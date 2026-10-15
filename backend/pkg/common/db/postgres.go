package db

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
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

// NewPostgresDialector constructs a gorm.Dialector with PreferSimpleProtocol set to true.
// This ensures compatibility with both standard PostgreSQL (:5432) and transaction poolers
// like Supabase Supavisor (:6543) or PgBouncer by avoiding extended protocol prepared statement caching.
func NewPostgresDialector(dsn string) gorm.Dialector {
	return postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true,
	})
}

// ConfigurePool configures connection pool limits on a *sql.DB instance.
// Designed to keep multi-service deployments within cloud pool limits (e.g. Supabase
// session mode pool limit of 15 connections shared across microservices).
//
// Environment variable overrides:
// - DB_MAX_OPEN_CONNS: integer > 0 (defaults to defaultMaxOpen, usually 3-4)
// - DB_MAX_IDLE_CONNS: integer >= 0 (defaults to defaultMaxIdle, usually 2)
// - DB_CONN_MAX_LIFETIME: duration e.g. "30m" (defaults to 30m)
// - DB_CONN_MAX_IDLE_TIME: duration e.g. "2m" (defaults to 2m)
func ConfigurePool(sqlDB *sql.DB, defaultMaxOpen, defaultMaxIdle int) {
	if sqlDB == nil {
		return
	}

	maxOpen := defaultMaxOpen
	if envVal := os.Getenv("DB_MAX_OPEN_CONNS"); envVal != "" {
		if v, err := strconv.Atoi(envVal); err == nil && v > 0 {
			maxOpen = v
		}
	}

	maxIdle := defaultMaxIdle
	if envVal := os.Getenv("DB_MAX_IDLE_CONNS"); envVal != "" {
		if v, err := strconv.Atoi(envVal); err == nil && v >= 0 {
			maxIdle = v
		}
	}
	if maxIdle > maxOpen {
		maxIdle = maxOpen
	}

	maxLifetime := 30 * time.Minute
	if envVal := os.Getenv("DB_CONN_MAX_LIFETIME"); envVal != "" {
		if d, err := time.ParseDuration(envVal); err == nil && d > 0 {
			maxLifetime = d
		}
	}

	maxIdleTime := 2 * time.Minute
	if envVal := os.Getenv("DB_CONN_MAX_IDLE_TIME"); envVal != "" {
		if d, err := time.ParseDuration(envVal); err == nil && d > 0 {
			maxIdleTime = d
		}
	}

	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(maxLifetime)
	sqlDB.SetConnMaxIdleTime(maxIdleTime)
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
		db, err = gorm.Open(NewPostgresDialector(dsn), DefaultGormConfig())
		if err == nil {
			log.Println("Connected to PostgreSQL successfully!")
			if sqlDB, dbErr := db.DB(); dbErr == nil {
				ConfigurePool(sqlDB, 3, 2)
			}
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
