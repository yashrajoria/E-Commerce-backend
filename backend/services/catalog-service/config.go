package main

import (
	"fmt"
	"os"
	"time"
)

// Config holds all environment variables for catalog-service.
type Config struct {
	Port               string        // Service port (default: 8082)
	JWTSecret          string        // JWT secret
	RedisURL           string        // Redis (product cache + cart + bulk queue)
	S3Prefix           string        // Image storage prefix (default: products/)
	CartTTL            time.Duration
}

// LoadConfig loads environment variables into Config struct and validates them.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Port:      os.Getenv("PORT"),
		JWTSecret: os.Getenv("JWT_SECRET"),
		RedisURL:  os.Getenv("REDIS_URL"),
		CartTTL:   7 * 24 * time.Hour,
	}
	if cfg.Port == "" {
		cfg.Port = "8082"
	}
	if cfg.RedisURL == "" {
		cfg.RedisURL = "redis://redis:6379"
	}

	cfg.S3Prefix = firstNonEmpty(os.Getenv("STORAGE_PREFIX"), os.Getenv("AWS_S3_PREFIX"), "products/")

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	return cfg, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
