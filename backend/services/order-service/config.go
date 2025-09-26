package main

import (
	"fmt"
	"os"
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
