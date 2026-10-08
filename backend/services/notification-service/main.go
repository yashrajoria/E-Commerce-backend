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
	commondb "github.com/yashrajoria/common/db"
	"github.com/yashrajoria/common/telemetry"
	"go.uber.org/zap"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()

	// --- OpenTelemetry (optional, no-op when OTEL_EXPORTER_OTLP_ENDPOINT unset) ---
	shutdownTelemetry := telemetry.Init(context.Background(), "notification-service")
	defer shutdownTelemetry()

	cfg, err := LoadConfig()
	if err != nil {
		logger.Fatal("Config load failed", zap.Error(err))
	}

	// Database
	if err := database.Connect(logger); err != nil {
		logger.Fatal("DB connection failed", zap.Error(err))
	}

	purgeCtx, stopPurge := context.WithCancel(context.Background())
	defer stopPurge()
	commondb.StartPurger(purgeCtx, database.DB, time.Hour,
		commondb.PurgeJob{Table: "notification_events", Where: "created_at < now() - interval '30 days'"},
	)

	// Senders
	emailSender, err := sender.NewSMTPSender()
	if err != nil {
		logger.Fatal("Failed to init SMTP sender", zap.Error(err))
	}

	// Dependency injection
	notificationRepo := repository.NewNotificationRepository(database.DB)
	notificationService, err := services.NewNotificationService(notificationRepo, emailSender, nil, logger)
	if err != nil {
		logger.Fatal("Failed to initialize notification service", zap.Error(err))
	}
	notificationController := controllers.NewNotificationController(notificationService, logger)

	// Postgres Queue Consumer
	sqsConsumer, err := consumer.NewSQSConsumerWithDB(database.DB, notificationService, logger)
	if err != nil {
		logger.Fatal("Failed to init queue consumer", zap.Error(err))
	}

	// Router
	r := gin.New()
	r.Use(gin.Recovery())
	// OpenTelemetry server spans (no-op when telemetry disabled) — before all
	// other middleware so traces span the full request lifecycle.
	r.Use(telemetry.GinMiddleware("notification-service"))

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
