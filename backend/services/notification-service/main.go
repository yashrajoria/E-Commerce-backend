package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"notification-service/consumer"
	"notification-service/controllers"
	"notification-service/database"
	"notification-service/repository"
	"notification-service/routes"
	"notification-service/sender"
	"notification-service/services"

	"github.com/gin-gonic/gin"
	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"go.uber.org/zap"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()

	cfg, err := LoadConfig()
	if err != nil {
		logger.Fatal("Config load failed", zap.Error(err))
	}

	// Database
	if err := database.Connect(logger); err != nil {
		logger.Fatal("DB connection failed", zap.Error(err))
	}

	// CloudWatch (non-fatal)
	metricsClient, err := aws_pkg.NewMetricsClient(context.Background())
	if err != nil {
		logger.Warn("CloudWatch metrics client init failed (non-fatal)", zap.Error(err))
	}

	// Senders
	emailSender, err := sender.NewSMTPSender()
	if err != nil {
		logger.Fatal("Failed to init SMTP sender", zap.Error(err))
	}
	// smsSender, err := sender.NewTwilioSender()
	// if err != nil {
	// 	logger.Fatal("Failed to init Twilio sender", zap.Error(err))
	// }

	// Dependency injection
	notificationRepo := repository.NewNotificationRepository(database.DB)
	// notificationService, err := services.NewNotificationService(notificationRepo, emailSender, smsSender, logger)
	notificationService, err := services.NewNotificationService(notificationRepo, emailSender, logger)
	if err != nil {
		logger.Fatal("Failed to initialize notification service", zap.Error(err))
	}
	notificationController := controllers.NewNotificationController(notificationService, logger)

	// SQS Consumer
	sqsConsumer, err := consumer.NewSQSConsumer(notificationService, logger)
	if err != nil {
		logger.Fatal("Failed to init SQS consumer", zap.Error(err))
	}

	// Router
	r := gin.New()
	r.Use(gin.Recovery())

	// CloudWatch middleware
	r.Use(func(c *gin.Context) {
		if metricsClient == nil || !metricsClient.IsEnabled() {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		dur := time.Since(start)
		go func() {
			mctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			dims := map[string]string{
				"Service": "notification-service",
				"Method":  c.Request.Method,
				"Path":    c.Request.URL.Path,
			}
			_ = metricsClient.RecordCount(mctx, aws_pkg.MetricHTTPRequests, dims)
			_ = metricsClient.RecordLatency(mctx, aws_pkg.MetricHTTPLatency, dur, dims)
			if c.Writer.Status() >= 400 {
				_ = metricsClient.RecordCount(mctx, aws_pkg.MetricHTTPErrors, dims)
			}
		}()
	})

	// Request logging
	r.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("http_request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
		)
	})

	// Request timeout
	r.Use(func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	routes.RegisterRoutes(r, notificationController)

	// Start SQS consumer
	consumerCtx, consumerCancel := context.WithCancel(context.Background())
	defer consumerCancel()
	go sqsConsumer.Start(consumerCtx)

	// HTTP server
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}
	go func() {
		logger.Info("Notification service started", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server failed", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Initiating graceful shutdown...")
	consumerCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
	}

	if err := database.Close(); err != nil {
		logger.Error("Database close error", zap.Error(err))
	}

	logger.Info("Notification service stopped gracefully")
}
