package main

import (
	"log"
	"user-service/database"
	"user-service/routes"

	// middlewares "user-service/middleware"
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"user-service/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {
	log.Println("Starting User Service...")
	// Initialize structured logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Load configuration
	cfg, err := LoadConfig()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	// Connect to the database
	if err := database.Connect(); err != nil {
		logger.Fatal("Database connection failed", zap.Error(err))
	}

	// Run migrations only if NOT in production
	if os.Getenv("ENV") != "production" {
		if err := models.Migrate(database.DB); err != nil {
			logger.Fatal("Migration failed", zap.Error(err))
		}
	}

	// Initialize Gin router
	r := gin.New()

	// Global middlewares
	r.Use(gin.Recovery()) // ✅ panic protection
	// r.Use(middlewares.SecurityHeaders())     // ✅ security headers
	// r.Use(middlewares.RateLimitMiddleware()) // ✅ rate limiting
	// r.Use(logger.RequestLogger())              // Add structured request logging if available

	// CORS
	allowedOrigins := map[string]bool{
		"http://localhost:3000":  true,
		"https://yourdomain.com": true,
	}

	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if !allowedOrigins[origin] {
			origin = "http://localhost:3000"
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		c.Header("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}
		c.Next()
	})

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "OK"})
	})

	routes.RegisterUserRoutes(r)

	// Port fallback
	port := cfg.Port
	if port == "" {
		port = "8085"
	}

	// Create HTTP server with timeout
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	// Graceful shutdown setup
	go func() {
		logger.Info("User Service started", zap.String("port", port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server error", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited cleanly")
}
