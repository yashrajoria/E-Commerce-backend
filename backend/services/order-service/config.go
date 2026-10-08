package main

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port                string
	PostgresUser        string
	PostgresPassword    string
	PostgresDB          string
	PostgresHost        string
	PostgresPort        string
	PostgresSSLMode     string
	PostgresTimeZone    string
	ProductServiceURL   string
	InventoryServiceURL string
	// Messaging config (backed by Postgres queues)
	CheckoutQueueURL        string
	PaymentEventsQueueURL   string
	PaymentRequestQueueURL  string
	OrderSNSTopicARN        string
	PaymentSNSTopicARN      string
	NotificationSNSTopicARN string
	StoreCurrency           string
	StripeSecretKey         string
	StripeWebhookSecret     string
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		Port:                    getEnv("PORT", "8083"),
		PostgresUser:            os.Getenv("POSTGRES_USER"),
		PostgresPassword:        os.Getenv("POSTGRES_PASSWORD"),
		PostgresDB:              os.Getenv("POSTGRES_DB"),
		PostgresHost:            os.Getenv("POSTGRES_HOST"),
		PostgresPort:            getEnv("POSTGRES_PORT", "5432"),
		PostgresSSLMode:         getEnv("POSTGRES_SSLMODE", "disable"),
		PostgresTimeZone:        getEnv("POSTGRES_TIMEZONE", "Asia/Kolkata"),
		ProductServiceURL:       getEnv("PRODUCT_SERVICE_URL", "http://catalog-service:8082"),
		InventoryServiceURL:     getEnv("INVENTORY_SERVICE_URL", "http://catalog-service:8082"),
		CheckoutQueueURL:        getEnv("CHECKOUT_QUEUE_URL", "order-processing-queue"),
		PaymentEventsQueueURL:   getEnv("PAYMENT_EVENTS_QUEUE_URL", "payment-events-queue"),
		PaymentRequestQueueURL:  getEnv("PAYMENT_REQUEST_QUEUE_URL", "payment-request-queue"),
		OrderSNSTopicARN:        getEnv("ORDER_SNS_TOPIC_ARN", "order-events"),
		PaymentSNSTopicARN:      getEnv("PAYMENT_SNS_TOPIC_ARN", "payment-events"),
		NotificationSNSTopicARN: getEnv("NOTIFICATION_SNS_TOPIC_ARN", "notification-queue"),
		StoreCurrency:           normalizeCurrency(getEnv("STORE_CURRENCY", "USD")),
		StripeSecretKey:         os.Getenv("STRIPE_API_KEY"),
		StripeWebhookSecret:     os.Getenv("STRIPE_WEBHOOK_SECRET"),
	}

	if cfg.PostgresUser == "" || cfg.PostgresPassword == "" || cfg.PostgresDB == "" || cfg.PostgresHost == "" {
		return nil, fmt.Errorf("database config incomplete")
	}
	if cfg.ProductServiceURL == "" {
		return nil, fmt.Errorf("PRODUCT_SERVICE_URL is required")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func normalizeCurrency(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "usd"
	}
	return normalized
}
