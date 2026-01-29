package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"product-service/controllers"
	"product-service/database"
	"product-service/repository"
	"product-service/routes"
	"product-service/services"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
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

	// Connect to the database
	if err := database.ConnectWithConfig(cfg.MongoURL, cfg.Database); err != nil {
		zap.L().Fatal("Failed to connect to database", zap.Error(err))
	}

	// Initialize AWS S3 (LocalStack-compatible) using AWS SDK v2
	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		awsRegion = "us-east-1"
	}
	awsEndpoint := os.Getenv("AWS_S3_ENDPOINT") // e.g. http://localstack:4566
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
				if service == s3.ServiceID {
					return aws.Endpoint{URL: awsEndpoint, SigningRegion: awsRegion}, nil
				}
				return aws.Endpoint{}, &aws.EndpointNotFoundError{}
			}),
		))
	}

	awsCfg, err := awscfg.LoadDefaultConfig(context.Background(), cfgOpts...)
	if err != nil {
		zap.L().Fatal("Failed to load AWS config", zap.Error(err))
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	// Presign client for generating presigned URLs
	presignClient := s3.NewPresignClient(s3Client)

	// --- 2. Dependency Injection (Wiring the layers together) ---

	// Initialize Repositories
	productRepo := repository.NewProductRepository(database.DB)
	categoryRepo := repository.NewCategoryRepository(database.DB)
	if err := productRepo.EnsureIndexes(context.Background()); err != nil {
		zap.L().Warn("Failed to ensure product indexes", zap.Error(err))
	}

	// Initialize Services, injecting repositories
	// Bucket and prefix (ensure these env vars are set; defaults provided)
	bucket := os.Getenv("AWS_S3_BUCKET")
	if bucket == "" {
		bucket = "shopswift"
	}
	prefix := os.Getenv("AWS_S3_PREFIX")
	if prefix == "" {
		prefix = "products/"
	}
	endpoint := os.Getenv("AWS_S3_ENDPOINT")
	cloudfrontDomain := os.Getenv("AWS_CLOUDFRONT_DOMAIN")

	productService := services.NewProductService(productRepo, categoryRepo, s3Client, presignClient, bucket, prefix, endpoint, cloudfrontDomain)
	categoryService := services.NewCategoryService(categoryRepo)

	// Initialize Controllers, injecting services
	productController := controllers.NewProductController(productService, ProductRedis)
	categoryController := controllers.NewCategoryController(categoryService)

	// --- 3. HTTP Server & Middleware ---

	r := gin.New()
	r.Use(gin.Recovery()) // Recover from panics

	// Add request timeout middleware
	r.Use(func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	// Add a request logger middleware here if desired

	// --- 4. Route Registration ---

	// Register all application routes, passing in the controllers
	routes.RegisterRoutes(r, productController, categoryController)

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

	// Close database connection
	if err := database.Close(); err != nil {
		zap.L().Error("Failed to close database", zap.Error(err))
	}

	// Close Redis connection
	if ProductRedis != nil {
		if err := ProductRedis.Close(); err != nil {
			zap.L().Error("Failed to close Redis", zap.Error(err))
		}
	}

	zap.L().Info("Product Service stopped gracefully")
}
