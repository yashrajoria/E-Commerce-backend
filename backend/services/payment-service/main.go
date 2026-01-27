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

	"payment-service/config"
	"payment-service/controllers"
	"payment-service/database"
	"payment-service/kafka"
	"payment-service/models"
	"payment-service/repository"
	"payment-service/routes"
	"payment-service/services"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal("[PaymentService] ❌ Failed to load config:", err)
	}

	// Connect DB
	if err := database.Connect(); err != nil {
		log.Fatal("[PaymentService] ❌ Failed to connect to DB:", err)
	}

	if err := database.DB.AutoMigrate(&models.Payment{}); err != nil {
		log.Fatal("[PaymentService] ❌ Failed to migrate Payment model:", err)
	}

	log.Println(cfg)

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatal("[PaymentService] ❌ Failed to initialize logger:", err)
	}
	defer logger.Sync()
	paymentRepo := repository.NewGormPaymentRepo(database.DB)
	// Stripe + Kafka setup
	stripeSvc := services.NewStripeService(cfg.StripeSecretKey, cfg.StripeWebhookKey)
	groupID := "payment-service-group" // use a unique group name
	paymentProducer := kafka.NewPaymentEventProducer(strings.Split(cfg.KafkaBrokers, ","), cfg.KafkaTopic)
	if paymentProducer == nil {
		logger.Fatal("Failed to create payment Kafka producer")
	}
	paymentRequestConsumer := services.NewPaymentRequestConsumer(
		strings.Split(cfg.KafkaBrokers, ","),
		cfg.PaymentRequestTopic,
		groupID,
		paymentProducer,
		stripeSvc,
		paymentRepo,
		logger,
	)
	// Start consuming payment requests in the background
	go paymentRequestConsumer.Start()

	defer paymentProducer.Close()

	// HTTP server
	r := gin.New()
	r.Use(gin.Recovery())

	pc := &controllers.PaymentController{
		Stripe: stripeSvc,
		Kafka:  paymentProducer,
		Repo:   paymentRepo,
		Logger: logger,
	}
	routes.RegisterPaymentRoutes(r, pc)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Start server in a goroutine
	go func() {
		logger.Info("[PaymentService] Running on port", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("[PaymentService] Server failed", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Info("Shutting down Payment Service...")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
	}

	// Close database connection
	sqlDB, _ := database.DB.DB()
	if sqlDB != nil {
		sqlDB.Close()
	}

	logger.Info("Payment Service stopped gracefully")
}
