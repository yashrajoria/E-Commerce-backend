package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port                   string
	PostgresUser           string
	PostgresPassword       string
	PostgresDB             string
	PostgresHost           string
	PostgresPort           string
	PostgresSSLMode        string
	PostgresTimeZone       string
	StripeSecretKey        string
	StripeWebhookKey       string
	PaymentRequestQueueURL string // SQS queue URL for payment requests
	PaymentSNSTopicARN     string // SNS topic ARN for payment events
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		Port:                   getEnv("PORT", "8087"),
		PostgresUser:           os.Getenv("POSTGRES_USER"),
		PostgresPassword:       os.Getenv("POSTGRES_PASSWORD"),
		PostgresDB:             os.Getenv("POSTGRES_DB"),
		PostgresHost:           os.Getenv("POSTGRES_HOST"),
		PostgresPort:           getEnv("POSTGRES_PORT", "5432"),
		PostgresSSLMode:        getEnv("POSTGRES_SSLMODE", "disable"),
		PostgresTimeZone:       getEnv("POSTGRES_TIMEZONE", "Asia/Kolkata"),
		StripeSecretKey:        os.Getenv("STRIPE_API_KEY"),
		StripeWebhookKey:       os.Getenv("STRIPE_WEBHOOK_SECRET"),
		PaymentRequestQueueURL: os.Getenv("PAYMENT_REQUEST_QUEUE_URL"),
		PaymentSNSTopicARN:     getEnv("PAYMENT_SNS_TOPIC_ARN", "arn:aws:sns:eu-west-2:000000000000:payment-events"),
	}

	if cfg.PostgresUser == "" || cfg.PostgresPassword == "" || cfg.PostgresDB == "" || cfg.PostgresHost == "" ||
		cfg.StripeSecretKey == "" || cfg.StripeWebhookKey == "" {
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
