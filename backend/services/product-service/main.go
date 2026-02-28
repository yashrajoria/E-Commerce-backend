package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"product-service/controllers"
	"product-service/repository"
	"product-service/routes"
	"product-service/services"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
	awspkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	commonmw "github.com/yashrajoria/common/middleware"
	"go.uber.org/zap"
)

var ProductRedis *redis.Client

func main() {
	// Initialize structured logger
	logger, err := zap.NewProduction()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()        // Flushes buffer, if any
	zap.ReplaceGlobals(logger) // Set the global logger

	// Load .env file (optional, falls back to system env)
	_ = godotenv.Load()

	// --- 1. Initialization ---
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://redis:6379"
	}
	redisOpts, err := redis.ParseURL(redisURL)
	if err != nil {
		zap.L().Warn("Failed to parse REDIS_URL, falling back to default", zap.Error(err))
		redisOpts = &redis.Options{Addr: "redis:6379", DB: 0}
	}
	ProductRedis = redis.NewClient(redisOpts)

	// Load configuration from environment variables
	cfg, err := LoadConfig()
	if err != nil {
		zap.L().Fatal("Failed to load configuration", zap.Error(err))
	}

	// Initialize AWS configuration using shared loader
	awsCfg, err := awspkg.LoadAWSConfig(context.Background())
	if err != nil {
		zap.L().Fatal("Failed to load AWS config", zap.Error(err))
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true // Important for LocalStack compatibility
	})
	// Presign client for generating presigned URLs
	presignClient := s3.NewPresignClient(s3Client)

	// --- 2. Dependency Injection (Wiring the layers together) ---

	// Bucket and prefix (ensure these env vars are set; defaults provided)
	bucket := os.Getenv("AWS_S3_BUCKET")
	if bucket == "" {
		bucket = "shopswift"
	}
	prefix := os.Getenv("AWS_S3_PREFIX")
	if prefix == "" {
		prefix = "products/"
	}
	endpoint := ""
	cloudfrontDomain := os.Getenv("AWS_CLOUDFRONT_DOMAIN")

	ddbClient := dynamodb.NewFromConfig(awsCfg)

	// Products table
	ddbTable := os.Getenv("DDB_TABLE_PRODUCTS")
	if ddbTable == "" {
		ddbTable = "Products"
	}
	productRepo := repository.NewDynamoAdapter(ddbClient, ddbTable)
	if err := productRepo.EnsureIndexes(context.Background()); err != nil {
		zap.L().Warn("Failed to ensure product indexes", zap.Error(err))
	}

	// Categories table
	ddbCategoryTable := os.Getenv("DDB_TABLE_CATEGORIES")
	if ddbCategoryTable == "" {
		ddbCategoryTable = "Categories"
	}
	categoryRepo := repository.NewDynamoCategoryAdapter(ddbClient, ddbCategoryTable, ddbTable)

	// Inventory client for syncing stock on product creation
	inventoryURL := os.Getenv("INVENTORY_SERVICE_URL")
	if inventoryURL == "" {
		inventoryURL = "http://inventory-service:8084"
	}
	inventoryClient := services.NewInventoryClient(inventoryURL)

	// Initialize Services using DynamoDB repositories
	productService := services.NewProductServiceDDB(productRepo, categoryRepo, s3Client, presignClient, bucket, prefix, endpoint, cloudfrontDomain, inventoryClient)
	categoryService := services.NewCategoryServiceDDB(categoryRepo, productRepo)

	// Initialize Controllers, injecting services
	productController := controllers.NewProductController(productService, ProductRedis)
	categoryController := controllers.NewCategoryController(categoryService)

	// Start bulk import worker (consumes persisted files from storage and processes them)
	storageDir := os.Getenv("BULK_STORAGE_DIR")
	if storageDir == "" {
		storageDir = "./data/bulk_imports"
	}
	services.StartBulkImportWorker(context.Background(), ProductRedis, productService, storageDir)

	// --- CloudWatch (Logs + Metrics) ---
	cwLogsClient, err := awspkg.NewCloudWatchLogsClient(context.Background(), "product-service")
	if err != nil {
		zap.L().Warn("CloudWatch logs client init failed (non-fatal)", zap.Error(err))
	}
	_ = cwLogsClient // available for future Zap WriteSyncer integration

	metricsClient, err := awspkg.NewMetricsClient(context.Background())
	if err != nil {
		zap.L().Warn("CloudWatch metrics client init failed (non-fatal)", zap.Error(err))
	}

	// --- 3. HTTP Server & Middleware ---

	r := gin.New()
	r.Use(gin.Recovery()) // Recover from panics

	// CloudWatch HTTP metrics middleware
	if metricsClient != nil {
		r.Use(commonmw.MetricsMiddleware(metricsClient, "product-service"))
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

	// --- 4. Route Registration ---

	// Register all application routes, passing in the controllers
	routes.RegisterRoutesLegacy(r, productController, categoryController)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "OK"})
	})

	// --- 5. Graceful Shutdown ---

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		zap.L().Info("Product Service starting", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.L().Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for an interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	zap.L().Info("Shutting down Product Service...")

	// Create a context with a timeout to allow for cleanup
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		zap.L().Fatal("Server forced to shutdown", zap.Error(err))
	}

	// Close Redis connection
	if ProductRedis != nil {
		if err := ProductRedis.Close(); err != nil {
			zap.L().Error("Failed to close Redis", zap.Error(err))
		}
	}

	zap.L().Info("Product Service stopped gracefully")
}
