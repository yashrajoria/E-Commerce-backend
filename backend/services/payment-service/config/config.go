package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port             string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	PostgresHost     string
	PostgresPort     string
	PostgresSSLMode  string
	PostgresTimeZone string
	StripeSecretKey  string
	StripeWebhookKey string
	KafkaBrokers     string
	KafkaTopic       string
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		Port:             getEnv("PORT", "8087"),
		PostgresUser:     os.Getenv("POSTGRES_USER"),
		PostgresPassword: os.Getenv("POSTGRES_PASSWORD"),
		PostgresDB:       os.Getenv("POSTGRES_DB"),
		PostgresHost:     os.Getenv("POSTGRES_HOST"),
		PostgresPort:     getEnv("POSTGRES_PORT", "5432"),
		PostgresSSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
		PostgresTimeZone: getEnv("POSTGRES_TIMEZONE", "Asia/Kolkata"),
		StripeSecretKey:  os.Getenv("STRIPE_API_KEY"),
		StripeWebhookKey: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		KafkaBrokers:     os.Getenv("KAFKA_BROKERS"),
		KafkaTopic:       os.Getenv("PAYMENT_KAFKA_TOPIC"),
	}

	if cfg.PostgresUser == "" || cfg.PostgresPassword == "" || cfg.PostgresDB == "" || cfg.PostgresHost == "" ||
		cfg.StripeSecretKey == "" || cfg.StripeWebhookKey == "" || cfg.KafkaBrokers == "" {
		return nil, fmt.Errorf("missing required environment variables")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
