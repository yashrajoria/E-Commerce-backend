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
	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	commondb "github.com/yashrajoria/common/db"
	apperrors "github.com/yashrajoria/common/errors"
	"github.com/yashrajoria/common/internalauth"
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

	if err := database.Connect(); err != nil {
		logger.Fatal("DB connection failed", zap.Error(err))
	}
	if commondb.AllowAutoMigrate() {
		if err := database.DB.AutoMigrate(&models.Order{}, &models.OrderItem{}, &promotionmodels.Coupon{}, &paymentmodels.Payment{}, &paymentmodels.StripeProcessedEvent{}, &paymentmodels.OutboxEvent{}); err != nil {
			logger.Fatal("Migration failed", zap.Error(err))
		}
	}

	// --- AWS setup ---
	awsCfg, err := aws_pkg.LoadAWSConfig(context.Background())
	if err != nil {
		logger.Fatal("Failed to load AWS config", zap.Error(err))
	}

	// SNS client for publishing order events
	snsClient := aws_pkg.NewSNSClient(awsCfg)

	// --- HTTP router ---
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(apperrors.ErrorMiddleware())
	r.Use(middleware.ConfigMiddleware(cfg.ProductServiceURL))

	// CloudWatch HTTP metrics middleware (metricsClient created later, use closure)
	var metricsClient *aws_pkg.MetricsClient
	r.Use(func(c *gin.Context) {
		if metricsClient == nil || !metricsClient.IsEnabled() {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		go func(path, method string, status int, dur time.Duration) {
			mctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			dims := map[string]string{"Service": "order-service", "Method": method, "Path": path}
			_ = metricsClient.RecordCount(mctx, aws_pkg.MetricHTTPRequests, dims)
			_ = metricsClient.RecordLatency(mctx, aws_pkg.MetricHTTPLatency, dur, dims)
			if status >= 400 {
				_ = metricsClient.RecordCount(mctx, aws_pkg.MetricHTTPErrors, dims)
			}
		}(c.Request.URL.Path, c.Request.Method, c.Writer.Status(), time.Since(start))
	})

	// Structured HTTP request logging → CloudWatch via Zap writer
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
		snsClient,
		cfg.OrderSNSTopicARN, // reuse order SNS for promotion events (or add dedicated topic)
		cfg.NotificationSNSTopicARN,
		logger,
	)
	shippingProvider := shippingproviders.NewInternalDynamicProvider()
	shippingService := shippingservices.NewShippingService(shippingProvider, logger, cfg.StoreCurrency)

	// --- Get queue URLs early (needed for payment services) ---
	checkoutQueueURL := cfg.CheckoutQueueURL
	if checkoutQueueURL == "" {
		if url, err := aws_pkg.GetQueueURL(context.Background(), awsCfg, "order-processing-queue"); err == nil {
			checkoutQueueURL = url
		} else {
			logger.Warn("Could not get checkout queue URL", zap.Error(err))
		}
	}

	paymentEventsQueueURL := cfg.PaymentEventsQueueURL
	if paymentEventsQueueURL == "" {
		if url, err := aws_pkg.GetQueueURL(context.Background(), awsCfg, "payment-events-queue"); err == nil {
			paymentEventsQueueURL = url
		} else {
			logger.Warn("Could not get payment events queue URL", zap.Error(err))
		}
	}

	paymentRequestQueueURL := cfg.PaymentRequestQueueURL
	if paymentRequestQueueURL == "" {
		if url, err := aws_pkg.GetQueueURL(context.Background(), awsCfg, "payment-request-queue"); err == nil {
			paymentRequestQueueURL = url
		} else {
			logger.Warn("Could not get payment request queue URL", zap.Error(err))
		}
	}

	// Fail fast in production when messaging is unconfigured — degraded
	// start (Warn + serve HTTP) is intentional for dev/LocalStack only.
	if os.Getenv("ENV") == "production" {
		var missing []string
		if checkoutQueueURL == "" {
			missing = append(missing, "CHECKOUT_QUEUE_URL/order-processing-queue")
		}
		if paymentRequestQueueURL == "" {
			missing = append(missing, "PAYMENT_REQUEST_QUEUE_URL/payment-request-queue")
		}
		if cfg.OrderSNSTopicARN == "" {
			missing = append(missing, "ORDER_SNS_TOPIC_ARN")
		}
		if len(missing) > 0 {
			logger.Fatal("missing required messaging config in production", zap.Strings("missing", missing))
		}
	}

	// Payment services
	stripeSvc := paymentservices.NewStripeService(cfg.StripeSecretKey, cfg.StripeWebhookSecret)
	paymentRepo := paymentrepository.NewGormPaymentRepo(database.DB)
	paymentOutboxRepo := paymentrepository.NewGormOutboxRepository(database.DB)
	paymentOutboxPublisher := paymentservices.NewOutboxPublisher(
		paymentOutboxRepo,
		snsClient,
		"order-service-payment-outbox-"+uuid.NewString(),
	)
	paymentRequestConsumer := paymentservices.NewPaymentRequestConsumer(
		aws_pkg.NewSQSConsumer(awsCfg, paymentRequestQueueURL),
		snsClient,
		cfg.PaymentSNSTopicARN,
		cfg.NotificationSNSTopicARN,
		stripeSvc,
		cfg.StoreCurrency,
		paymentRepo,
		logger,
	)

	orderService := services.NewOrderServiceSQS(
		orderRepository,
		snsClient,
		cfg.OrderSNSTopicARN,
		cfg.NotificationSNSTopicARN,
		inventoryClient,
	)
	orderController := controllers.NewOrderController(orderService)

	// Promotion controller & routes
	promoController := promotioncontrollers.NewCouponController(promoService)
	shippingController := shippingcontrollers.NewShippingController(shippingService)
	paymentController := paymentcontrollers.NewPaymentController(stripeSvc, snsClient, cfg.PaymentSNSTopicARN, cfg.NotificationSNSTopicARN, cfg.StoreCurrency, paymentRepo, logger)
	routes.RegisterOrderRoutes(r, orderController)
	promotionroutes.RegisterCouponRoutes(r, promoController)
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

	// --- SQS Consumers (replaces Kafka) ---
	if paymentRequestQueueURL != "" || cfg.NotificationSNSTopicARN != "" {
		outboxPublisher := services.NewOutboxPublisher(
			outboxRepository,
			aws_pkg.NewSQSOutboxPublisher(awsCfg),
			snsClient,
			"order-service-"+uuid.NewString(),
		)
		go outboxPublisher.Run(shutdownCtx)
		logger.Info("Started outbox publisher", zap.String("payment_queue", paymentRequestQueueURL))
	} else {
		logger.Warn("Outbox publisher not started - payment request queue URL is missing")
	}

	// Start payment outbox publisher (for payment webhook events)
	if cfg.PaymentSNSTopicARN != "" {
		go paymentOutboxPublisher.Run(shutdownCtx)
		logger.Info("Started payment outbox publisher")
	} else {
		logger.Warn("Payment outbox publisher not started - payment SNS topic ARN missing")
	}

	// Inventory client for stock management
	inventoryClient = services.NewInventoryClient(cfg.InventoryServiceURL)

	// CloudWatch Metrics
	cwLogsClient, cloudwatchErr := aws_pkg.NewCloudWatchLogsClient(context.Background(), "order-service")
	if cloudwatchErr != nil {
		logger.Warn("CloudWatch logs client init failed (non-fatal)", zap.Error(cloudwatchErr))
	}
	_ = cwLogsClient

	metricsClient, err = aws_pkg.NewMetricsClient(context.Background())
	if err != nil {
		logger.Warn("CloudWatch metrics client init failed (non-fatal)", zap.Error(err))
	}

	// Promotion client for coupon validation (now in-process)
	promotionClient := promoService

	// Start SQS consumers
	if checkoutQueueURL != "" && paymentRequestQueueURL != "" {
		checkoutConsumer := services.NewSQSCheckoutConsumer(
			aws_pkg.NewSQSConsumer(awsCfg, checkoutQueueURL),
			aws_pkg.NewSQSConsumer(awsCfg, paymentRequestQueueURL), // For sending payment requests
			orderRepository,
			inventoryClient,
			metricsClient,
			cfg.ProductServiceURL,
			snsClient,
			cfg.NotificationSNSTopicARN,
			promotionClient,
			cfg.StoreCurrency,
		)
		go checkoutConsumer.Start(shutdownCtx)
		logger.Info("Started SQS checkout consumer", zap.String("queue", checkoutQueueURL))
	} else {
		logger.Warn("Checkout consumer not started - missing queue URLs")
	}

	// Start payment request consumer
	if paymentRequestQueueURL != "" {
		go paymentRequestConsumer.Start(shutdownCtx)
		logger.Info("Started SQS payment request consumer", zap.String("queue", paymentRequestQueueURL))
	} else {
		logger.Warn("Payment request consumer not started - missing queue URL")
	}

	if paymentEventsQueueURL != "" {
		paymentConsumer := services.NewSQSPaymentConsumer(
			aws_pkg.NewSQSConsumer(awsCfg, paymentEventsQueueURL),
			orderRepository,
			inventoryClient,
			metricsClient,
			snsClient,
			cfg.NotificationSNSTopicARN,
			cfg.ProductServiceURL,
		)
		go paymentConsumer.Start(shutdownCtx)
		logger.Info("Started SQS payment events consumer", zap.String("queue", paymentEventsQueueURL))
	} else {
		logger.Warn("Payment events consumer not started - missing queue URL")
	}

	// Coupon usage consumer (absorbed from promotion-service): increments
	// coupon used_count on order_created. Queue is SNS-fanned-out from the
	// notification topic by LocalStack bootstrap / Terraform.
	orderCreatedQueueURL := os.Getenv("ORDER_CREATED_QUEUE_URL")
	if orderCreatedQueueURL == "" {
		if url, err := aws_pkg.GetQueueURL(context.Background(), awsCfg, "promotion-order-queue"); err == nil {
			orderCreatedQueueURL = url
		} else {
			logger.Warn("Could not get order-created queue URL", zap.Error(err))
		}
	}
	if orderCreatedQueueURL != "" {
		orderCreatedConsumer := promotionconsumer.NewOrderCreatedConsumer(
			aws_pkg.NewSQSConsumer(awsCfg, orderCreatedQueueURL),
			promoService,
		)
		go orderCreatedConsumer.Start(shutdownCtx)
		logger.Info("Started SQS order-created (coupon usage) consumer", zap.String("queue", orderCreatedQueueURL))
	} else {
		logger.Warn("Order-created consumer not started - missing queue URL (coupon usage will not increment)")
	}

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
