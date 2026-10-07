package main

import (
	"context"
	"fmt"
	"os"
	"time"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
)

// Config holds all environment variables for catalog-service.
type Config struct {
	Port               string // Service port (default: 8082, kept from product-service)
	JWTSecret          string // JWT secret (required by product + inventory auth)
	RedisURL           string // Redis (product cache + cart + bulk queue)
	S3Bucket           string
	S3Prefix           string
	AssetPublicBaseURL string
	CloudFrontDomain   string
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

	cfg.S3Bucket = firstNonEmpty(os.Getenv("AWS_S3_BUCKET"), os.Getenv("S3_BUCKET_IMAGES"), "shopswift")
	cfg.S3Prefix = firstNonEmpty(os.Getenv("AWS_S3_PREFIX"), "products/")
	cfg.AssetPublicBaseURL = os.Getenv("ASSET_PUBLIC_BASE_URL")
	cfg.CloudFrontDomain = os.Getenv("AWS_CLOUDFRONT_DOMAIN")

	if cfg.AssetPublicBaseURL == "" && aws_pkg.IsLocalStack() {
		cfg.AssetPublicBaseURL = "http://localhost:4566"
	}

	if os.Getenv("AWS_USE_SECRETS") == "true" {
		if awsCfg, err := aws_pkg.LoadAWSConfig(context.Background()); err == nil {
			sm := aws_pkg.NewSecretsClient(awsCfg)
			if jwt, err := sm.GetSecret(context.Background(), "product/JWT_SECRET"); err == nil && jwt != "" {
				cfg.JWTSecret = jwt
			}
		}
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	return cfg, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
