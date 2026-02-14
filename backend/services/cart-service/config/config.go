package config

import (
	"os"
	"time"
)

type Config struct {
	Port             string
	RedisURL         string
	CartTTL          time.Duration
	CheckoutQueueURL string // SQS queue URL for checkout events
	OrderSNSTopicARN string // SNS topic ARN for order events
}

func Load() Config {
	return Config{
		Port:             getEnv("PORT", "8086"),
		RedisURL:         getEnv("REDIS_URL", "redis://redis:6379"),
		CartTTL:          time.Hour * 24 * 7, // default 7 days
		CheckoutQueueURL: os.Getenv("CHECKOUT_QUEUE_URL"),
		OrderSNSTopicARN: getEnv("ORDER_SNS_TOPIC_ARN", "arn:aws:sns:eu-west-2:000000000000:order-events"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
