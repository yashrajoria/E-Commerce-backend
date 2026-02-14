package main

import (
	"context"
	"fmt"
	"os"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
)

// Config holds all environment variables for the product-service.
type Config struct {
	JWTSecret string // JWT secret for authentication
	Port      string // Service port (default: 8082)
}

// LoadConfig loads environment variables into Config struct and validates them.
// If AWS_USE_SECRETS=true it will attempt to read secrets from Secrets Manager
// and fall back to env vars on failure.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		JWTSecret: os.Getenv("JWT_SECRET"),
		Port:      os.Getenv("PORT"),
	}

	// Set default port if not provided
	if cfg.Port == "" {
		cfg.Port = "8082"
	}

	if os.Getenv("AWS_USE_SECRETS") == "true" {
		if awsCfg, err := aws_pkg.LoadAWSConfig(context.Background()); err == nil {
			sm := aws_pkg.NewSecretsClient(awsCfg)

			if jwt, err := sm.GetSecret(context.Background(), "product/JWT_SECRET"); err == nil && jwt != "" {
				cfg.JWTSecret = jwt
			}
		}
	}

	// Validate required fields
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	return cfg, nil
}
