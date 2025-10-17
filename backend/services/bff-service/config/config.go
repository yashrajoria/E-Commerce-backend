package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Config holds the loaded configuration
type Config struct {
	BffAddr         string
	UserServiceURL  string
	OrderServiceURL string
}

var AppConfig Config

// LoadConfig loads configuration from the .env file
func LoadConfig() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	AppConfig = Config{
		BffAddr:         getEnv("BFF_SERVICE_ADDR", ":8000"),
		UserServiceURL:  getEnv("USER_SERVICE_URL", ""),
		OrderServiceURL: getEnv("ORDER_SERVICE_URL", ""),
	}
}

// Helper to get an environment variable or return a default
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	if fallback == "" {
		log.Fatalf("FATAL: Environment variable %s is not set.", key)
	}
	return fallback
}
