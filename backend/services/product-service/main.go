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

	"github.com/cloudinary/cloudinary-go"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

var ProductRedis *redis.Client

func main() {
	// Initialize structured logger
	logger, _ := zap.NewProduction()
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

	// Initialize external services like Cloudinary
	// (Ensure CLOUDINARY_URL is in your .env or environment)
	cld, err := cloudinary.New()
	if err != nil {
		zap.L().Fatal("Failed to initialize Cloudinary", zap.Error(err))
	}

	// --- 2. Dependency Injection (Wiring the layers together) ---

	// Initialize Repositories
	productRepo := repository.NewProductRepository(database.DB)
	categoryRepo := repository.NewCategoryRepository(database.DB)
	if err := productRepo.EnsureIndexes(context.Background()); err != nil {
		zap.L().Warn("Failed to ensure product indexes", zap.Error(err))
	}

	// Initialize Services, injecting repositories
	productService := services.NewProductService(productRepo, categoryRepo, cld)
	categoryService := services.NewCategoryService(categoryRepo)

	// Initialize Controllers, injecting services
	productController := controllers.NewProductController(productService, ProductRedis)
	categoryController := controllers.NewCategoryController(categoryService)

	// --- 3. HTTP Server & Middleware ---

	r := gin.New()
	r.Use(gin.Recovery()) // Recover from panics
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		zap.L().Fatal("Server forced to shutdown", zap.Error(err))
	}

	zap.L().Info("Product Service stopped gracefully")
}
