package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"promotion-service/controllers"
	"promotion-service/database"
	"promotion-service/models"
	"promotion-service/repository"
	"promotion-service/routes"
	"promotion-service/services"

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

	// --- Database ---
	if err := database.Connect(); err != nil {
		logger.Fatal("DB connection failed", zap.Error(err))
	}
	if err := database.DB.AutoMigrate(&models.Coupon{}); err != nil {
		logger.Fatal("Migration failed", zap.Error(err))
	}

	// --- AWS setup ---
	awsCfg, err := aws_pkg.LoadAWSConfig(context.Background())
	if err != nil {
		logger.Fatal("Failed to load AWS config", zap.Error(err))
	}
	snsClient := aws_pkg.NewSNSClient(awsCfg)

	// --- HTTP router ---
	r := gin.New()
	r.Use(gin.Recovery())

	// CloudWatch HTTP metrics middleware
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
			dims := map[string]string{"Service": "promotion-service", "Method": method, "Path": path}
			_ = metricsClient.RecordCount(mctx, aws_pkg.MetricHTTPRequests, dims)
			_ = metricsClient.RecordLatency(mctx, aws_pkg.MetricHTTPLatency, dur, dims)
			if status >= 400 {
				_ = metricsClient.RecordCount(mctx, aws_pkg.MetricHTTPErrors, dims)
			}
		}(c.Request.URL.Path, c.Request.Method, c.Writer.Status(), time.Since(start))
	})

	// Structured HTTP request logging
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

	// Request timeout middleware
	r.Use(func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	// --- Dependency injection ---
	couponRepo := repository.NewGormCouponRepository(database.DB)
	couponService := services.NewCouponService(couponRepo, snsClient, cfg.PromotionSNSTopicARN, logger)
	couponController := controllers.NewCouponController(couponService)

	routes.RegisterCouponRoutes(r, couponController)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "OK", "service": "promotion-service"})
	})

	// --- CloudWatch metrics (non-fatal) ---
	metricsClient, err = aws_pkg.NewMetricsClient(context.Background())
	if err != nil {
		logger.Warn("CloudWatch metrics client init failed (non-fatal)", zap.Error(err))
	}

	// --- HTTP server ---
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}
	go func() {
		logger.Info("Promotion Service started", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server failed", zap.Error(err))
		}
	}()

	// --- Graceful shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Initiating graceful shutdown...")
	httpShutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(httpShutdownCtx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
	}

	if err := database.Close(); err != nil {
		logger.Error("Database close error", zap.Error(err))
	}

	log.Println("Promotion Service stopped gracefully")
}
