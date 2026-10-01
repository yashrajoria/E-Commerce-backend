package main

import (
	"fmt"
	"os"
)

// Config holds the environment for identity-service.
type Config struct {
	// Service port (default: 8081, kept from auth-service so existing
	// AUTH_SERVICE_URL/USER_SERVICE_URL overrides and gateway defaults keep working).
	Port string
}

// LoadConfig loads and validates environment variables.
// JWT_SECRET is required: TokenService panics without it, so fail fast here
// with a clear message instead. (The old user-service also demanded SMTP_*
// here, but nothing in either service ever read them — dropped.)
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Port: os.Getenv("PORT"),
	}
	if cfg.Port == "" {
		cfg.Port = "8081"
	}
	if os.Getenv("JWT_SECRET") == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}
	return cfg, nil
}
