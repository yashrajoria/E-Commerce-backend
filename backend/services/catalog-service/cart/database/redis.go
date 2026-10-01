package database

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient initializes and returns a Redis client.
// Accepts both redis:// URIs and bare host:port (e.g. REDIS_URL=redis:6379).
func NewRedisClient(redisURL string) *redis.Client {
	if redisURL == "" {
		redisURL = "redis://redis:6379"
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Printf("REDIS_URL %q is not a URI, using as host:port: %v", redisURL, err)
		opts = &redis.Options{Addr: redisURL}
	}

	client := redis.NewClient(opts)

	// Optionally test the connection
	if err := client.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	log.Println("Connected to Redis")
	return client
}
