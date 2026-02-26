package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"shipping-service/models"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
)

// Config holds all configuration for the shipping service.
type Config struct {
	Port                string
	PostgresUser        string
	PostgresPassword    string
	PostgresDB          string
	PostgresHost        string
	PostgresPort        string
	PostgresSSLMode     string
	PostgresTimeZone    string
	ShippoAPIKey        string
	ShippingSNSTopicARN string
	// Warehouse / origin address defaults
	OriginName       string
	OriginStreet1    string
	OriginCity       string
	OriginState      string
	OriginPostalCode string
	OriginCountry    string
	OriginPhone      string
}

// OriginAddress builds an Address struct from origin config values.
func (c *Config) OriginAddress() models.Address {
	return models.Address{
		Name:       c.OriginName,
		Street1:    c.OriginStreet1,
		City:       c.OriginCity,
		State:      c.OriginState,
		PostalCode: c.OriginPostalCode,
		Country:    c.OriginCountry,
		Phone:      c.OriginPhone,
	}
}

// LoadConfig reads configuration from environment variables with optional
// Secrets Manager override.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Port:                getEnv("PORT", "8091"),
		PostgresUser:        os.Getenv("POSTGRES_USER"),
		PostgresPassword:    os.Getenv("POSTGRES_PASSWORD"),
		PostgresDB:          os.Getenv("POSTGRES_DB"),
		PostgresHost:        os.Getenv("POSTGRES_HOST"),
		PostgresPort:        getEnv("POSTGRES_PORT", "5432"),
		PostgresSSLMode:     getEnv("POSTGRES_SSLMODE", "disable"),
		PostgresTimeZone:    getEnv("POSTGRES_TIMEZONE", "Asia/Kolkata"),
		ShippoAPIKey:        os.Getenv("SHIPPO_API_KEY"),
		ShippingSNSTopicARN: os.Getenv("SHIPPING_SNS_TOPIC_ARN"),
		// Origin (warehouse) address
		OriginName:       getEnv("ORIGIN_NAME", "ShopSwift Warehouse"),
		OriginStreet1:    getEnv("ORIGIN_STREET1", "123 Warehouse Blvd"),
		OriginCity:       getEnv("ORIGIN_CITY", "San Francisco"),
		OriginState:      getEnv("ORIGIN_STATE", "CA"),
		OriginPostalCode: getEnv("ORIGIN_POSTAL_CODE", "94105"),
		OriginCountry:    getEnv("ORIGIN_COUNTRY", "US"),
		OriginPhone:      getEnv("ORIGIN_PHONE", "+14155550100"),
	}

	// Override DB credentials from Secrets Manager when running on AWS
	if os.Getenv("AWS_USE_SECRETS") == "true" {
		if awsCfg, err := aws_pkg.LoadAWSConfig(context.Background()); err == nil {
			sm := aws_pkg.NewSecretsClient(awsCfg)
			if dbjson, err := sm.GetSecret(context.Background(), "shipping/DB_CREDENTIALS"); err == nil && dbjson != "" {
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
			if v, err := sm.GetSecret(context.Background(), "shipping/SHIPPO_API_KEY"); err == nil && v != "" {
				cfg.ShippoAPIKey = v
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
