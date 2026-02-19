package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"user-service/database"
	"user-service/middleware"
	"user-service/models"
	"user-service/routes"

	"github.com/gin-gonic/gin"
	awspkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	commonmw "github.com/yashrajoria/common/middleware"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	log.Println("Starting User Service...")

	cfg, err := LoadConfig()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	if err := database.Connect(); err != nil {
		logger.Fatal("Database connection failed", zap.Error(err))
	}

	if os.Getenv("ENV") != "production" {
		if err := models.Migrate(database.DB); err != nil {
			logger.Fatal("Migration failed", zap.Error(err))
		}
	}

	r := gin.New()
	r.Use(gin.Recovery())

	// --- CloudWatch (Logs + Metrics) ---
	cwLogsClient, err := awspkg.NewCloudWatchLogsClient(context.Background(), "user-service")
	if err != nil {
		logger.Warn("CloudWatch logs client init failed (non-fatal)", zap.Error(err))
	}
	_ = cwLogsClient

	metricsClient, err := awspkg.NewMetricsClient(context.Background())
	if err != nil {
		logger.Warn("CloudWatch metrics client init failed (non-fatal)", zap.Error(err))
	}

	// CloudWatch HTTP metrics middleware
	if metricsClient != nil {
		r.Use(commonmw.MetricsMiddleware(metricsClient, "user-service"))
	}

	// Structured HTTP request logging → CloudWatch via Zap writer
	r.Use(commonmw.RequestLogger(logger))

	// Add request timeout middleware
	r.Use(func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	// CORS is handled by API Gateway, not here
	// Remove duplicate CORS middleware to avoid conflicts

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "OK"})
	})

	// User routes with authentication middleware
	userRoutes := r.Group("/users")
	userRoutes.Use(middleware.AuthMiddleware())
	routes.RegisterUserRoutes(userRoutes)

	port := cfg.Port
	if port == "" {
		port = "8085"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		logger.Info("User Service started", zap.String("port", port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server error", zap.Error(err))
		}
	}()

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
