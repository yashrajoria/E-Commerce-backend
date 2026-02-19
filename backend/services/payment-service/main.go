package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"payment-service/config"
	"payment-service/controllers"
	"payment-service/database"
	"payment-service/models"
	"payment-service/repository"
	"payment-service/routes"
	"payment-service/services"

	"github.com/gin-gonic/gin"
	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	commonmw "github.com/yashrajoria/common/middleware"
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

	// AWS setup
	awsCfg, err := aws_pkg.LoadAWSConfig(context.Background())
	if err != nil {
		logger.Fatal("Failed to load AWS config", zap.Error(err))
	}

	// SNS publisher for payment events
	paymentTopicArn := os.Getenv("PAYMENT_SNS_TOPIC_ARN")
	if paymentTopicArn == "" {
		paymentTopicArn = "arn:aws:sns:eu-west-2:000000000000:payment-events"
	}
	snsPublisher := aws_pkg.NewSNSClient(awsCfg)

	// SQS consumer for payment requests
	paymentQueueURL := os.Getenv("PAYMENT_REQUEST_QUEUE_URL")
	if paymentQueueURL == "" {
		// Get queue URL from LocalStack
		queueURL, err := aws_pkg.GetQueueURL(context.Background(), awsCfg, "payment-request-queue")
		if err != nil {
			logger.Warn("Could not get payment queue URL", zap.Error(err))
			paymentQueueURL = "http://localhost:4566/000000000000/payment-request-queue"
		} else {
			paymentQueueURL = queueURL
		}
	}

	stripeSvc := services.NewStripeService(cfg.StripeSecretKey, cfg.StripeWebhookKey)
	sqsConsumer := aws_pkg.NewSQSConsumer(awsCfg, paymentQueueURL)
	paymentRequestConsumer := services.NewPaymentRequestConsumer(
		sqsConsumer,
		snsPublisher,
		paymentTopicArn,
		stripeSvc,
		paymentRepo,
		logger,
	)

	// --- Graceful shutdown context ---
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	// Start consuming payment requests in the background
	go paymentRequestConsumer.Start(shutdownCtx)

	// --- CloudWatch (Logs + Metrics) ---
	cwLogsClient, err := aws_pkg.NewCloudWatchLogsClient(context.Background(), "payment-service")
	if err != nil {
		logger.Warn("CloudWatch logs client init failed (non-fatal)", zap.Error(err))
	}
	_ = cwLogsClient

	metricsClient, err := aws_pkg.NewMetricsClient(context.Background())
	if err != nil {
		logger.Warn("CloudWatch metrics client init failed (non-fatal)", zap.Error(err))
	}

	// HTTP server
	r := gin.New()
	r.Use(gin.Recovery())

	// CloudWatch HTTP metrics middleware
	if metricsClient != nil {
		r.Use(commonmw.MetricsMiddleware(metricsClient, "payment-service"))
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

	pc := &controllers.PaymentController{
		Stripe:   stripeSvc,
		SNS:      snsPublisher,
		TopicArn: paymentTopicArn,
		Repo:     paymentRepo,
		Logger:   logger,
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

	logger.Info("Initiating graceful shutdown...")
	shutdownCancel()            // Cancel consumer
	time.Sleep(1 * time.Second) // Give consumer time to shut down

	httpShutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Info("Shutting down Payment Service...")
	if err := srv.Shutdown(httpShutdownCtx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
	}

	// Close database connection
	sqlDB, _ := database.DB.DB()
	if sqlDB != nil {
		sqlDB.Close()
	}

	logger.Info("Payment Service stopped gracefully")
}
