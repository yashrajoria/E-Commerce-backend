package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"cart-service/config"
	"cart-service/database"
	"cart-service/routes"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
)

func main() {

	// Load environment configuration
	cfg := config.Load()

	// Initialize Redis client
	redisClient := database.NewRedisClient(cfg.RedisURL)

	// Initialize AWS SNS client
	awsCfg, err := aws_pkg.LoadAWSConfig(context.Background())
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}
	snsClient := aws_pkg.NewSNSClient(awsCfg)

	// Initialize Gin router
	router := gin.Default()

	// Register routes
	routes.RegisterCartRoutes(router, redisClient, snsClient, cfg)

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
