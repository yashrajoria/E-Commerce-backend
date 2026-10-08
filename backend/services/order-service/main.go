package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"order-service/controllers"
	"order-service/database"
	"order-service/middleware"
	"order-service/models"
	adminroutes "order-service/admin"
	repositories "order-service/repository"
	"order-service/routes"
	"order-service/services"
	paymentcontrollers "order-service/payment/controllers"
	paymentmodels "order-service/payment/models"
	paymentrepository "order-service/payment/repository"
	paymentroutes "order-service/payment/routes"
	paymentservices "order-service/payment/services"
	promotioncontrollers "order-service/promotion/controllers"
	promotionconsumer "order-service/promotion/consumer"
	promotionroutes "order-service/promotion/routes"
	promotionmodels "order-service/promotion/models"
	promotionrepository "order-service/promotion/repository"
	promotionservices "order-service/promotion/services"
	shippingcontrollers "order-service/shipping/controllers"
	shippingroutes "order-service/shipping/routes"
	shippingproviders "order-service/shipping/providers"
	shippingservices "order-service/shipping/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	commondb "github.com/yashrajoria/common/db"
	apperrors "github.com/yashrajoria/common/errors"
	"github.com/yashrajoria/common/internalauth"
	"github.com/yashrajoria/common/messaging"
	"github.com/yashrajoria/common/telemetry"
	"go.uber.org/zap"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	if internalauth.Token() == "" {
		logger.Warn("INTERNAL_SERVICE_TOKEN is not set — internal-only calls to/from order-service will be rejected")
	}

	cfg, err := LoadConfig()
	if err != nil {
		logger.Fatal("Config load failed", zap.Error(err))
	}

	// --- OpenTelemetry (optional, no-op when OTEL_EXPORTER_OTLP_ENDPOINT unset) ---
	shutdownTelemetry := telemetry.Init(context.Background(), "order-service")
	defer shutdownTelemetry()

	if err := database.Connect(); err != nil {
		logger.Fatal("DB connection failed", zap.Error(err))
	}
	if commondb.AllowAutoMigrate() {
		if err := database.DB.AutoMigrate(&models.Order{}, &models.OrderItem{}, &models.OutboxEvent{}, &promotionmodels.Coupon{}, &promotionmodels.CouponUsage{}, &paymentmodels.Payment{}, &paymentmodels.StripeProcessedEvent{}, &paymentmodels.OutboxEvent{}); err != nil {
			logger.Fatal("Migration failed", zap.Error(err))
		}
	}

	// --- Message Publisher (PostgreSQL queue backed) ---
	pgPublisher := messaging.NewPGQueuePublisher(database.DB, logger)

	// --- HTTP router ---
	r := gin.New()
	r.Use(gin.Recovery())
	// OpenTelemetry server spans (no-op when telemetry disabled) — before all
	// other middleware so traces span the full request lifecycle.
	r.Use(telemetry.GinMiddleware("order-service"))
	r.Use(apperrors.ErrorMiddleware())
	r.Use(middleware.ConfigMiddleware(cfg.ProductServiceURL))

	// Structured HTTP request logging via Zap
	r.Use(func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method
		c.Next()
		latency := time.Since(start)
		status := c.Writer.Status()
		fields := []zap.Field{
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
			zap.Int("body_size", c.Writer.Size()),
		}
		switch {
		case status >= 500:
			logger.Error("http_request", fields...)
		case status >= 400:
			logger.Warn("http_request", fields...)
		default:
			logger.Info("http_request", fields...)
		}
	})

	// Add request timeout middleware
	r.Use(func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	orderRepository := repositories.NewGormOrderRepository(database.DB)
	outboxRepository := repositories.NewGormOutboxRepository(database.DB)

	// Inventory client for stock management (also used by OrderService to
	// release reservations on admin cancellation).
	inventoryClient := services.NewInventoryClient(cfg.InventoryServiceURL)

	// --- Promotion, Shipping & Payment services (merged in-process) ---
	promoRepo := promotionrepository.NewGormCouponRepository(database.DB)
	promoService := promotionservices.NewCouponService(
		promoRepo,
		pgPublisher,
		cfg.OrderSNSTopicARN, // reuse order topic for promotion events
		cfg.NotificationSNSTopicARN,
		logger,
	)
	shippingProvider := shippingproviders.NewInternalDynamicProvider()
	shippingService := shippingservices.NewShippingService(shippingProvider, logger, cfg.StoreCurrency)

	// --- Queue names ---
	checkoutQueueURL := cfg.CheckoutQueueURL
	paymentEventsQueueURL := cfg.PaymentEventsQueueURL
	paymentRequestQueueURL := cfg.PaymentRequestQueueURL

	// Payment services
	stripeSvc := paymentservices.NewStripeService(cfg.StripeSecretKey, cfg.StripeWebhookSecret)
	paymentRepo := paymentrepository.NewGormPaymentRepo(database.DB)
	paymentOutboxRepo := paymentrepository.NewGormOutboxRepository(database.DB)
	paymentOutboxPublisher := paymentservices.NewOutboxPublisher(
		paymentOutboxRepo,
		pgPublisher,
		"order-service-payment-outbox-"+uuid.NewString(),
	)
	paymentRequestConsumer := paymentservices.NewPaymentRequestConsumer(
		messaging.NewPGQueueConsumer(database.DB, paymentRequestQueueURL, logger),
		pgPublisher,
		cfg.PaymentSNSTopicARN,
		cfg.NotificationSNSTopicARN,
		stripeSvc,
		cfg.StoreCurrency,
		paymentRepo,
		logger,
	)

	orderService := services.NewOrderServiceSQS(
		orderRepository,
		pgPublisher,
		cfg.OrderSNSTopicARN,
		cfg.NotificationSNSTopicARN,
		inventoryClient,
	)
	orderController := controllers.NewOrderController(orderService)

	// Promotion controller & routes
	promoController := promotioncontrollers.NewCouponController(promoService)
	basketOptService := promotionservices.NewBasketOptimizerService(logger)
	basketOptController := promotioncontrollers.NewBasketOptimizerController(basketOptService)
	shippingController := shippingcontrollers.NewShippingController(shippingService)
	paymentController := paymentcontrollers.NewPaymentController(stripeSvc, pgPublisher, cfg.PaymentSNSTopicARN, cfg.NotificationSNSTopicARN, cfg.StoreCurrency, paymentRepo, logger)
	routes.RegisterOrderRoutes(r, orderController)
	promotionroutes.RegisterPromotionRoutes(r, promoController, basketOptController)

	shippingroutes.RegisterShippingRoutes(r, shippingController)
	paymentroutes.RegisterPaymentRoutes(r, paymentController)
	adminroutes.RegisterDashboardRoutes(r, adminroutes.NewDashboardController(orderRepository, logger))

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/health/live", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/health/ready", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ready"}) })
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}

	// --- Graceful shutdown context ---
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	// Retention: published outbox rows and Stripe dedupe ids.
	commondb.StartPurger(shutdownCtx, database.DB, time.Hour,
		commondb.PurgeJob{Table: "outbox_events", Where: "status = 'published' AND published_at < now() - interval '7 days'"},
		commondb.PurgeJob{Table: "payment_outbox_events", Where: "status = 'published' AND published_at < now() - interval '7 days'"},
		commondb.PurgeJob{Table: "stripe_processed_events", Where: "processed_at < now() - interval '30 days'"},
	)

	// --- Outbox Publisher ---
	outboxPublisher := services.NewOutboxPublisher(
		outboxRepository,
		pgPublisher,
		pgPublisher,
		"order-service-"+uuid.NewString(),
	)
	go outboxPublisher.Run(shutdownCtx)
	logger.Info("Started outbox publisher", zap.String("payment_queue", paymentRequestQueueURL))

	// Start payment outbox publisher (for payment webhook events)
	go paymentOutboxPublisher.Run(shutdownCtx)
	logger.Info("Started payment outbox publisher")

	// Promotion client for coupon validation (in-process)
	promotionClient := promoService

	// Start Postgres queue consumers
	checkoutConsumer := services.NewSQSCheckoutConsumer(
		messaging.NewPGQueueConsumer(database.DB, checkoutQueueURL, logger),
		nil,
		orderRepository,
		inventoryClient,
		nil,
		cfg.ProductServiceURL,
		pgPublisher,
		cfg.NotificationSNSTopicARN,
		promotionClient,
		cfg.StoreCurrency,
	)
	go checkoutConsumer.Start(shutdownCtx)
	logger.Info("Started checkout consumer", zap.String("queue", checkoutQueueURL))

	// Start payment request consumer
	go paymentRequestConsumer.Start(shutdownCtx)
	logger.Info("Started payment request consumer", zap.String("queue", paymentRequestQueueURL))

	// Start payment events consumer
	paymentConsumer := services.NewSQSPaymentConsumer(
		messaging.NewPGQueueConsumer(database.DB, paymentEventsQueueURL, logger),
		orderRepository,
		inventoryClient,
		nil,
		pgPublisher,
		cfg.NotificationSNSTopicARN,
		cfg.ProductServiceURL,
	)
	go paymentConsumer.Start(shutdownCtx)
	logger.Info("Started payment events consumer", zap.String("queue", paymentEventsQueueURL))

	// Coupon usage consumer: increments coupon used_count on order_created.
	orderCreatedQueueURL := os.Getenv("ORDER_CREATED_QUEUE_URL")
	if orderCreatedQueueURL == "" {
		orderCreatedQueueURL = "promotion-order-queue"
	}
	orderCreatedConsumer := promotionconsumer.NewOrderCreatedConsumer(
		messaging.NewPGQueueConsumer(database.DB, orderCreatedQueueURL, logger),
		promoService,
	)
	go orderCreatedConsumer.Start(shutdownCtx)
	logger.Info("Started order-created (coupon usage) consumer", zap.String("queue", orderCreatedQueueURL))

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

	log.Println("Order Service stopped gracefully")
}
