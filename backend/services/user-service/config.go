package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
)

// Config holds all environment variables for the auth-service.
type Config struct {
	PostgresUser     string // PostgreSQL username
	PostgresPassword string // PostgreSQL password
	PostgresDB       string // PostgreSQL database name
	PostgresHost     string // PostgreSQL host
	PostgresPort     string // PostgreSQL port
	PostgresSSLMode  string // PostgreSQL SSL mode
	PostgresTimeZone string // PostgreSQL timezone
	JWTSecret        string // JWT secret for authentication
	SMTPEmail        string // SMTP email for sending mail
	SMTPPassword     string // SMTP password for sending mail
	Port             string // Service port (default: 8081)
}

// LoadConfig loads environment variables into Config struct and validates them.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		PostgresUser:     os.Getenv("POSTGRES_USER"),
		PostgresPassword: os.Getenv("POSTGRES_PASSWORD"),
		PostgresDB:       os.Getenv("POSTGRES_DB"),
		PostgresHost:     os.Getenv("POSTGRES_HOST"),
		PostgresPort:     os.Getenv("POSTGRES_PORT"),
		PostgresSSLMode:  os.Getenv("POSTGRES_SSLMODE"),
		PostgresTimeZone: os.Getenv("POSTGRES_TIMEZONE"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		SMTPEmail:        os.Getenv("SMTP_EMAIL"),
		SMTPPassword:     os.Getenv("SMTP_PASSWORD"),
		Port:             os.Getenv("PORT"),
	}

	if cfg.Port == "" {
		cfg.Port = "8085"
	}

	if os.Getenv("AWS_USE_SECRETS") == "true" {
		if awsCfg, err := aws_pkg.LoadAWSConfig(context.Background()); err == nil {
			sm := aws_pkg.NewSecretsClient(awsCfg)

			if jwt, err := sm.GetSecret(context.Background(), "user/JWT_SECRET"); err == nil && jwt != "" {
				cfg.JWTSecret = jwt
			}

			if dbjson, err := sm.GetSecret(context.Background(), "user/DB_CREDENTIALS"); err == nil && dbjson != "" {
				var m map[string]string
				if err := json.Unmarshal([]byte(dbjson), &m); err == nil {
					if v, ok := m["POSTGRES_USER"]; ok && v != "" {
						cfg.PostgresUser = v
					}
					if v, ok := m["POSTGRES_PASSWORD"]; ok && v != "" {
						cfg.PostgresPassword = v
					}
					if v, ok := m["POSTGRES_DB"]; ok && v != "" {
						cfg.PostgresDB = v
					}
					if v, ok := m["POSTGRES_HOST"]; ok && v != "" {
						cfg.PostgresHost = v
					}
					if v, ok := m["POSTGRES_PORT"]; ok && v != "" {
						cfg.PostgresPort = v
					}
				}
			}
		}
	}

	// Validate required fields
	if cfg.PostgresUser == "" {
		return nil, fmt.Errorf("POSTGRES_USER is required")
	}
	if cfg.PostgresPassword == "" {
		return nil, fmt.Errorf("POSTGRES_PASSWORD is required")
	}
	if cfg.PostgresDB == "" {
		return nil, fmt.Errorf("POSTGRES_DB is required")
	}
	if cfg.PostgresHost == "" {
		return nil, fmt.Errorf("POSTGRES_HOST is required")
	}
	if cfg.PostgresPort == "" {
		return nil, fmt.Errorf("POSTGRES_PORT is required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}
	if cfg.SMTPEmail == "" {
		return nil, fmt.Errorf("SMTP_EMAIL is required")
	}
	if cfg.SMTPPassword == "" {
		return nil, fmt.Errorf("SMTP_PASSWORD is required")
	}

	return cfg, nil
}
