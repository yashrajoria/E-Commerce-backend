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

	"order-service/controllers"
	"order-service/database"
	"order-service/kafka"
	"order-service/middleware"
	"order-service/models"
	repositories "order-service/repository"
	"order-service/routes"
	"order-service/services"

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

	if err := database.Connect(); err != nil {
		logger.Fatal("DB connection failed", zap.Error(err))
	}
	if err := database.DB.AutoMigrate(&models.Order{}, &models.OrderItem{}); err != nil {
		logger.Fatal("Migration failed", zap.Error(err))
	}
	// --- HTTP router ---
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ConfigMiddleware(cfg.ProductServiceURL))

	// Add request timeout middleware
	r.Use(func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	orderRepository := repositories.NewGormOrderRepository(database.DB)

	// Optional SNS setup (used for order events push to LocalStack/AWS)
	var snsClient *aws_pkg.SNSClient
	snsTopic := os.Getenv("ORDER_SNS_TOPIC_ARN")
	if snsTopic != "" {
		if awsCfg, err := aws_pkg.LoadAWSConfig(context.Background()); err == nil {
			snsClient = aws_pkg.NewSNSClient(awsCfg)
		} else {
			logger.Warn("Failed to load AWS config for SNS", zap.Error(err))
		}
	}

	orderService := services.NewOrderService(
		orderRepository,
		kafka.NewProducer(strings.Split(cfg.KafkaBrokers, ","), cfg.KafkaTopic),
		cfg.KafkaTopic,
		snsClient,
		os.Getenv("ORDER_SNS_TOPIC_ARN"),
	)
	orderController := controllers.NewOrderController(orderService)
	routes.RegisterOrderRoutes(r, orderController)

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "OK"}) })
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}

	// --- Kafka config (env w/ sensible defaults for Docker) ---
	brokersEnv := os.Getenv("KAFKA_BROKERS")
	log.Println("KAFKA_BROKERS:", brokersEnv)
	if brokersEnv == "" {
		brokersEnv = "kafka:9092" // works inside docker-compose network
	}
	// Allow both "kafka:9092" or "kafka:9092,another:9092"
	brokers := strings.Split(brokersEnv, ",")

	checkoutTopic := cfg.KafkaTopic
	paymentEventsTopic := os.Getenv("PAYMENT_TOPIC")
	if paymentEventsTopic == "" {
		paymentEventsTopic = os.Getenv("PAYMENT_KAFKA_TOPIC")
		if paymentEventsTopic == "" {
			paymentEventsTopic = "payment-events"
		}
	}
	paymentRequestsTopic := os.Getenv("PAYMENT_REQUEST_TOPIC")
	if paymentRequestsTopic == "" {
		paymentRequestsTopic = "payment-requests"
	}
	groupID := "order-service-group"

	// --- Producer: payment-requests (Order → Payment) ---
	paymentProducer := kafka.NewProducer(brokers, paymentRequestsTopic)
	if paymentProducer == nil {
		logger.Fatal("Failed to create payment Kafka producer")
	}
	defer paymentProducer.Close()

	// --- Graceful shutdown context ---
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	// --- Consumer: checkout-events (Cart → Order) ---
	go services.StartCheckoutConsumer(shutdownCtx, brokers, checkoutTopic, groupID, database.DB, paymentProducer)

	// --- Consumer: payment-events (Payment → Order) ---
	paymentConsumer := services.NewPaymentConsumer(brokers, paymentEventsTopic, groupID, database.DB)
	go paymentConsumer.Start()
	defer paymentConsumer.Close()

	// --- HTTP server ---
	go func() {
		logger.Info("Order Service started", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server failed", zap.Error(err))
		}
	}()

	// --- Graceful shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Initiating graceful shutdown...")
	shutdownCancel()            // Cancel all consumers
	time.Sleep(1 * time.Second) // Give consumers time to shut down

	httpShutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Info("Shutting down Order Service...")
	if err := srv.Shutdown(httpShutdownCtx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
	}

	// Close database connection
	sqlDB, _ := database.DB.DB()
	if sqlDB != nil {
		sqlDB.Close()
	}

	logger.Info("Order Service stopped gracefully")
}
