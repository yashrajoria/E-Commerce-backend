package config

import "os"

type Config struct {
	Port           string
	APIGatewayURL  string
	RequestTimeout string
}

func Load() Config {
	return Config{
		Port:           getEnv("PORT", "8088"),
		APIGatewayURL:  getEnv("API_GATEWAY_URL", "http://api-gateway:8080"),
		RequestTimeout: getEnv("REQUEST_TIMEOUT", "10s"),
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
