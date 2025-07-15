package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	// "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/gin-gonic/gin"

	"cart-service/config"
	"cart-service/database"

	"cart-service/kafka"
	"cart-service/routes"
)

func main() {
	go kafka.StartCheckoutConsumer("kafka:9092", "checkout", "order-service-group")

	// Load environment configuration
	cfg := config.Load()

	// Initialize Redis client
	redisClient := database.NewRedisClient(cfg.RedisURL)

	// Initialize Kafka producer
	producer, err := kafka.NewProducer(strings.Split(cfg.KafkaBrokers, ","), cfg.KafkaTopic)
	if err != nil {
		log.Fatalf("failed to create Kafka producer: %v", err)
	}
	defer producer.Close()

	// Initialize Gin router
	router := gin.Default()

	// Register routes
	routes.RegisterCartRoutes(router, redisClient, producer, cfg)

	// Start HTTP server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	go func() {
		log.Printf("Cart Service is running on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Shutdown error: %v", err)
	}
	log.Println("Server shutdown complete.")
}
