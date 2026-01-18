package config

import (
	"os"
	"time"
)

type Config struct {
	Port         string
	RedisURL     string
	KafkaBrokers string
	KafkaTopic   string
	CartTTL      time.Duration
}

func Load() Config {
	return Config{
		Port:         getEnv("PORT", "8086"),
		RedisURL:     getEnv("REDIS_URL", "redis://redis:6379"),
		KafkaBrokers: os.Getenv("KAFKA_BROKERS"),
		KafkaTopic:   getEnv("CHECKOUT_TOPIC", getEnv("CART_KAFKA_TOPIC", "checkout.requested")),
		CartTTL:      time.Hour * 24 * 7, // default 7 days
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
