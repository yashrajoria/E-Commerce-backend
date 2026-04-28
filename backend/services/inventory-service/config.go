package main

import (
	"context"
	"fmt"
	"os"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
)

// Config holds all configuration for the inventory-service.
type Config struct {
	Port      string // Service port (default: 8084)
	JWTSecret string // JWT secret for authentication
	DDBTable  string // DynamoDB table name for inventory
}

// LoadConfig loads environment variables into Config struct.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Port:      os.Getenv("PORT"),
		JWTSecret: os.Getenv("JWT_SECRET"),
		DDBTable:  os.Getenv("DDB_TABLE_INVENTORY"),
	}

	if cfg.Port == "" {
		cfg.Port = "8084"
	}
	if cfg.DDBTable == "" {
		cfg.DDBTable = "Inventory"
	}

	if os.Getenv("AWS_USE_SECRETS") == "true" {
		if awsCfg, err := aws_pkg.LoadAWSConfig(context.Background()); err == nil {
			sm := aws_pkg.NewSecretsClient(awsCfg)

			if jwt, err := sm.GetSecret(context.Background(), "inventory/JWT_SECRET"); err == nil && jwt != "" {
				cfg.JWTSecret = jwt
			}
		}
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	return cfg, nil
}
