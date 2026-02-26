package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"shipping-service/controllers"
	"shipping-service/database"
	"shipping-service/providers"
	"shipping-service/repository"
	"shipping-service/routes"
	servicepkg "shipping-service/services"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"go.uber.org/zap"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Failed to init logger: %v", err)
	}
	defer logger.Sync() //nolint:errcheck

	cfg, err := LoadConfig()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	if err := database.Connect(); err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer database.Close() //nolint:errcheck

	// AWS clients
	awsCfg, awsErr := aws_pkg.LoadAWSConfig(context.Background())
	var snsClient aws_pkg.SNSPublisher

	if awsErr != nil {
		logger.Warn("AWS config unavailable, SNS disabled", zap.Error(awsErr))
	} else {
		snsClient = aws_pkg.NewSNSClient(awsCfg)
	}

	// Provider and DI chain
	shippingProvider := providers.NewShippoProvider(cfg.ShippoAPIKey)
	shipmentRepo := repository.NewGormShipmentRepository(database.DB)
	shippingService := servicepkg.NewShippingService(
		shipmentRepo,
		shippingProvider,
		snsClient,
		cfg.ShippingSNSTopicARN,
		cfg.OriginAddress(),
		logger,
	)
	shippingController := controllers.NewShippingController(shippingService)

	r := gin.New()

	// Global request-logging middleware
	r.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)
		logger.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("duration", duration),
		)
	})

	// 30-second request timeout
	r.Use(func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "service": "shipping-service"})
	})

	routes.RegisterShippingRoutes(r, shippingController)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server failed", zap.Error(err))
		}
	}()

	logger.Info("Shipping service started", zap.String("port", cfg.Port))
	<-quit
	logger.Info("Shutting down shipping service...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}
	logger.Info("Server exited cleanly")
}
