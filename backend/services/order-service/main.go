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

	orderRepository := repositories.NewGormOrderRepository(database.DB)
	orderService := services.NewOrderService(
		orderRepository,
		kafka.NewProducer(strings.Split(cfg.KafkaBrokers, ","), cfg.KafkaTopic),
		cfg.KafkaTopic,
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

	// --- Consumer: checkout-events (Cart → Order) ---
	go services.StartCheckoutConsumer(brokers, checkoutTopic, groupID, database.DB, paymentProducer)

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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	logger.Info("Shutting down Order Service...")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
	}

	// Close database connection
	sqlDB, _ := database.DB.DB()
	if sqlDB != nil {
		sqlDB.Close()
	}

	logger.Info("Order Service stopped gracefully")
}
