package database

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient initializes and returns a Redis client
func NewRedisClient(redisURL string) *redis.Client {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("Invalid Redis URL: %v", err)
	}

	client := redis.NewClient(opts)

	// Optionally test the connection
	if err := client.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	log.Println("Connected to Redis")
	return client
}
