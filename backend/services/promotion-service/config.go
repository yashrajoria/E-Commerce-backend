package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
)

// Config holds all configuration for the promotion service.
type Config struct {
	Port             string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	PostgresHost     string
	PostgresPort     string
	PostgresSSLMode  string
	PostgresTimeZone string
	// SNS topic for promotion events
	PromotionSNSTopicARN string
}

// LoadConfig reads configuration from environment variables with optional
// Secrets Manager override.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Port:                 getEnv("PORT", "8090"),
		PostgresUser:         os.Getenv("POSTGRES_USER"),
		PostgresPassword:     os.Getenv("POSTGRES_PASSWORD"),
		PostgresDB:           os.Getenv("POSTGRES_DB"),
		PostgresHost:         os.Getenv("POSTGRES_HOST"),
		PostgresPort:         getEnv("POSTGRES_PORT", "5432"),
		PostgresSSLMode:      getEnv("POSTGRES_SSLMODE", "disable"),
		PostgresTimeZone:     getEnv("POSTGRES_TIMEZONE", "Asia/Kolkata"),
		PromotionSNSTopicARN: os.Getenv("PROMOTION_SNS_TOPIC_ARN"),
	}

	// Override DB credentials from Secrets Manager when running on AWS
	if os.Getenv("AWS_USE_SECRETS") == "true" {
		if awsCfg, err := aws_pkg.LoadAWSConfig(context.Background()); err == nil {
			sm := aws_pkg.NewSecretsClient(awsCfg)

			if dbjson, err := sm.GetSecret(context.Background(), "promotion/DB_CREDENTIALS"); err == nil && dbjson != "" {
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

	if cfg.PostgresUser == "" || cfg.PostgresPassword == "" || cfg.PostgresDB == "" || cfg.PostgresHost == "" {
		return nil, fmt.Errorf("database config incomplete")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
