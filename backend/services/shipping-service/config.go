package main

import (
	"os"
)

// Config holds runtime configuration for the shipping service.
type Config struct {
	Port string
}

// LoadConfig reads configuration from environment variables.
func LoadConfig() (*Config, error) {
	return &Config{
		Port: getEnv("PORT", "8091"),
	}, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
