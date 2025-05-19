package main

import (
	"fmt"
	"os"
)

// Config holds all environment variables for the product-service.
type Config struct {
	MongoURL    string // MongoDB connection string
	Database    string // MongoDB database name
	JWTSecret   string // JWT secret for authentication
	Port        string // Service port (default: 8082)
}

// LoadConfig loads environment variables into Config struct and validates them.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		MongoURL:  os.Getenv("MONGO_DB_URL"),
		Database:  os.Getenv("MONGO_DB_NAME"),
		JWTSecret: os.Getenv("JWT_SECRET"),
		Port:      os.Getenv("PORT"),
	}

	// Set default port if not provided
	if cfg.Port == "" {
		cfg.Port = "8082"
	}

	// Validate required fields
	if cfg.MongoURL == "" {
		return nil, fmt.Errorf("MONGO_DB_URL is required")
	}
	if cfg.Database == "" {
		return nil, fmt.Errorf("MONGO_DB_NAME is required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	return cfg, nil
} 