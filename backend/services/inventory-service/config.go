package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
)

// Config holds all environment variables for the inventory-service.
type Config struct {
	MongoURL    string // MongoDB connection string
	Database    string // MongoDB database name
	JWTSecret   string // JWT secret for authentication
	Port        string // Service port (default: 8084)
}

// LoadConfig loads environment variables into Config struct and validates them.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		MongoURL:  os.Getenv("MONGO_DB_URL"),
		Database:  os.Getenv("MONGO_DB_NAME"),
		JWTSecret: os.Getenv("JWT_SECRET"),
		Port:      os.Getenv("PORT"),
	}

	if cfg.Port == "" {
		cfg.Port = "8084"
	}

	if os.Getenv("AWS_USE_SECRETS") == "true" {
		if awsCfg, err := aws_pkg.LoadAWSConfig(context.Background()); err == nil {
			sm := aws_pkg.NewSecretsClient(awsCfg)

			if jwt, err := sm.GetSecret(context.Background(), "inventory/JWT_SECRET"); err == nil && jwt != "" {
				cfg.JWTSecret = jwt
			}

			if dbjson, err := sm.GetSecret(context.Background(), "inventory/DB_CREDENTIALS"); err == nil && dbjson != "" {
				var m map[string]string
				if err := json.Unmarshal([]byte(dbjson), &m); err == nil {
					if v, ok := m["MONGO_DB_URL"]; ok && v != "" {
						cfg.MongoURL = v
					}
					if v, ok := m["MONGO_DB_NAME"]; ok && v != "" {
						cfg.Database = v
					}
				}
			}
		}
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