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

	orderService := services.NewOrderServiceSQS(
		orderRepository,
		snsClient,
		cfg.OrderSNSTopicARN,
	)
	orderController := controllers.NewOrderController(orderService)
	routes.RegisterOrderRoutes(r, orderController)

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "OK"}) })
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}

	// --- Graceful shutdown context ---
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	// --- SQS Consumers (replaces Kafka) ---
	// Get queue URLs (fallback to env if not in config)
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

	// Inventory client for stock management
	inventoryClient := services.NewInventoryClient(cfg.InventoryServiceURL)

	// CloudWatch Metrics
	cwLogsClient, err := aws_pkg.NewCloudWatchLogsClient(context.Background(), "order-service")
	if err != nil {
		logger.Warn("CloudWatch logs client init failed (non-fatal)", zap.Error(err))
	}
	_ = cwLogsClient

	metricsClient, err = aws_pkg.NewMetricsClient(context.Background())
	if err != nil {
		logger.Warn("CloudWatch metrics client init failed (non-fatal)", zap.Error(err))
	}

	// Start SQS consumers
	if checkoutQueueURL != "" && paymentRequestQueueURL != "" {
		checkoutConsumer := services.NewSQSCheckoutConsumer(
			aws_pkg.NewSQSConsumer(awsCfg, checkoutQueueURL),
			aws_pkg.NewSQSConsumer(awsCfg, paymentRequestQueueURL), // For sending payment requests
			database.DB,
			inventoryClient,
			metricsClient,
			cfg.ProductServiceURL,
		)
		go checkoutConsumer.Start(shutdownCtx)
		logger.Info("Started SQS checkout consumer", zap.String("queue", checkoutQueueURL))
	} else {
		logger.Warn("Checkout consumer not started - missing queue URLs")
	}

	if paymentEventsQueueURL != "" {
		paymentConsumer := services.NewSQSPaymentConsumer(
			aws_pkg.NewSQSConsumer(awsCfg, paymentEventsQueueURL),
			database.DB,
			inventoryClient,
			metricsClient,
		)
		go paymentConsumer.Start(shutdownCtx)
		logger.Info("Started SQS payment events consumer", zap.String("queue", paymentEventsQueueURL))
	} else {
		logger.Warn("Payment events consumer not started - missing queue URL")
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
