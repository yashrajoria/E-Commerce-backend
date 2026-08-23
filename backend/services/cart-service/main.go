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
	"go.uber.org/zap"

	"cart-service/config"
	"cart-service/database"
	"cart-service/routes"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"github.com/yashrajoria/common/internalauth"
	commonmw "github.com/yashrajoria/common/middleware"
)

func main() {
	if internalauth.Token() == "" {
		log.Println("WARNING: INTERNAL_SERVICE_TOKEN is not set — internal-only calls to/from cart-service will be rejected")
	}

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

	// --- CloudWatch (Logs + Metrics) ---
	cwLogsClient, err := aws_pkg.NewCloudWatchLogsClient(context.Background(), "cart-service")
	if err != nil {
		log.Printf("CloudWatch logs client init failed (non-fatal): %v", err)
	}
	_ = cwLogsClient

	metricsClient, err := aws_pkg.NewMetricsClient(context.Background())
	if err != nil {
		log.Printf("CloudWatch metrics client init failed (non-fatal): %v", err)
	}

	// Initialize Gin router
	router := gin.Default()

	// Initialize structured logger for request logging
	zapLogger, _ := zap.NewProduction()
	defer zapLogger.Sync()

	// CloudWatch HTTP metrics middleware
	if metricsClient != nil {
		router.Use(commonmw.MetricsMiddleware(metricsClient, "cart-service"))
	}

	// Structured HTTP request logging → CloudWatch via Zap writer
	router.Use(commonmw.RequestLogger(zapLogger))

	// Register routes
	routes.RegisterCartRoutes(router, redisClient, snsClient, cfg)

	router.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	router.GET("/health/live", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	router.GET("/health/ready", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ready"}) })

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
