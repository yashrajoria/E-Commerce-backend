package main

import (
	"os"
)

// Config holds all configuration for the notification service.
type Config struct {
	Port             string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	PostgresHost     string
	PostgresPort     string
	PostgresSSLMode  string
	PostgresTimeZone string
	QueueURL         string
}

// LoadConfig reads configuration from environment variables.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Port:             getEnv("PORT", "8092"),
		PostgresUser:     os.Getenv("POSTGRES_USER"),
		PostgresPassword: os.Getenv("POSTGRES_PASSWORD"),
		PostgresDB:       os.Getenv("POSTGRES_DB"),
		PostgresHost:     os.Getenv("POSTGRES_HOST"),
		PostgresPort:     getEnv("POSTGRES_PORT", "5432"),
		PostgresSSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
		PostgresTimeZone: getEnv("POSTGRES_TIMEZONE", "UTC"),
		QueueURL:         getEnv("NOTIFICATION_SQS_QUEUE_URL", "notification-queue"),
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
