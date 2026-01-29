package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
)

type Config struct {
	Port              string
	PostgresUser      string
	PostgresPassword  string
	PostgresDB        string
	PostgresHost      string
	PostgresPort      string
	PostgresSSLMode   string
	PostgresTimeZone  string
	ProductServiceURL string
	KafkaBrokers      string
	KafkaTopic        string
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		Port:              getEnv("PORT", "8083"),
		PostgresUser:      os.Getenv("POSTGRES_USER"),
		PostgresPassword:  os.Getenv("POSTGRES_PASSWORD"),
		PostgresDB:        os.Getenv("POSTGRES_DB"),
		PostgresHost:      os.Getenv("POSTGRES_HOST"),
		PostgresPort:      getEnv("POSTGRES_PORT", "5432"),
		PostgresSSLMode:   getEnv("POSTGRES_SSLMODE", "disable"),
		PostgresTimeZone:  getEnv("POSTGRES_TIMEZONE", "Asia/Kolkata"),
		ProductServiceURL: getEnv("PRODUCT_SERVICE_URL", "http://product-service:8082"),
		KafkaBrokers:      os.Getenv("KAFKA_BROKERS"),
		KafkaTopic:        getEnv("CHECKOUT_TOPIC", getEnv("CART_KAFKA_TOPIC", "checkout.requested")),
	}

	if os.Getenv("AWS_USE_SECRETS") == "true" {
		if awsCfg, err := aws_pkg.LoadAWSConfig(context.Background()); err == nil {
			sm := aws_pkg.NewSecretsClient(awsCfg)

			if dbjson, err := sm.GetSecret(context.Background(), "order/DB_CREDENTIALS"); err == nil && dbjson != "" {
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
	if cfg.ProductServiceURL == "" {
		return nil, fmt.Errorf("PRODUCT_SERVICE_URL is required")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
