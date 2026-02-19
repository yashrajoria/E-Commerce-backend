package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/gin-gonic/gin"
	awspkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"github.com/yashrajoria/inventory-service/controllers"
	"github.com/yashrajoria/inventory-service/repository"
	"github.com/yashrajoria/inventory-service/routes"
	"github.com/yashrajoria/inventory-service/services"
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

	// --- AWS / DynamoDB setup ---
	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		awsRegion = "us-east-1"
	}
	awsEndpoint := os.Getenv("AWS_ENDPOINT")
	awsAccessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	awsSecret := os.Getenv("AWS_SECRET_ACCESS_KEY")

	cfgOpts := []func(*awscfg.LoadOptions) error{
		awscfg.WithRegion(awsRegion),
	}
	if awsAccessKey != "" || awsSecret != "" {
		cfgOpts = append(cfgOpts, awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(awsAccessKey, awsSecret, ""),
		))
	}
	if awsEndpoint != "" {
		cfgOpts = append(cfgOpts, awscfg.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{URL: awsEndpoint, SigningRegion: awsRegion}, nil
			}),
		))
	}

	awsCfg, err := awscfg.LoadDefaultConfig(context.Background(), cfgOpts...)
	if err != nil {
		logger.Fatal("Failed to load AWS config", zap.Error(err))
	}

	ddbClient := dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
		if awsEndpoint != "" {
			o.BaseEndpoint = aws.String(awsEndpoint)
		}
	})

	// --- Service wiring ---
	// CloudWatch (Logs + Metrics)
	cwLogsClient, err := awspkg.NewCloudWatchLogsClient(context.Background(), "inventory-service")
	if err != nil {
		logger.Warn("CloudWatch logs client init failed (non-fatal)", zap.Error(err))
	}
	metricsClient, err := awspkg.NewMetricsClient(context.Background())
	if err != nil {
		logger.Warn("CloudWatch metrics client init failed (non-fatal)", zap.Error(err))
	}
	_ = cwLogsClient // logs client available for future Zap integration

	inventoryRepo := repository.NewDynamoInventoryRepository(ddbClient, cfg.DDBTable)
	inventoryService := services.NewInventoryService(inventoryRepo, metricsClient)
	inventoryController := controllers.NewInventoryController(inventoryService)

	// --- HTTP router ---
	r := gin.New()
	r.Use(gin.Recovery())

	// CloudWatch HTTP metrics middleware
	if metricsClient != nil && metricsClient.IsEnabled() {
		r.Use(func(c *gin.Context) {
			start := time.Now()
			c.Next()
			go func(path, method string, status int, dur time.Duration) {
				mctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				dims := map[string]string{"Service": "inventory-service", "Method": method, "Path": path}
				_ = metricsClient.RecordCount(mctx, awspkg.MetricHTTPRequests, dims)
				_ = metricsClient.RecordLatency(mctx, awspkg.MetricHTTPLatency, dur, dims)
				if status >= 400 {
					_ = metricsClient.RecordCount(mctx, awspkg.MetricHTTPErrors, dims)
				}
			}(c.Request.URL.Path, c.Request.Method, c.Writer.Status(), time.Since(start))
		})
	}

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

	r.Use(func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	routes.RegisterRoutes(r, inventoryController)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "OK"})
	})

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}

	go func() {
		logger.Info("Inventory Service starting", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// --- Graceful shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down Inventory Service...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Inventory Service stopped gracefully")
}
