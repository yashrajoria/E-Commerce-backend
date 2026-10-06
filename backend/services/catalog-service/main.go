package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"catalog-service/controllers"
	inventorycontrollers "catalog-service/inventory/controllers"
	inventorymodels "catalog-service/inventory/models"
	inventoryrepository "catalog-service/inventory/repository"
	inventoryroutes "catalog-service/inventory/routes"
	inventoryservices "catalog-service/inventory/services"
	"catalog-service/repository"
	"catalog-service/routes"
	"catalog-service/services"
	cartroutes "catalog-service/cart/routes"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	awspkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"github.com/yashrajoria/common/internalauth"
	commonmw "github.com/yashrajoria/common/middleware"
	"github.com/yashrajoria/common/telemetry"
	"go.uber.org/zap"
)

// inventoryStockSync bridges product quantity changes into the inventory
// service in-process — the old product → inventory HTTP hop is gone.
// Semantics preserved: fire-and-forget, upsert-add, never fails the product op.
type inventoryStockSync struct {
	svc *inventoryservices.InventoryService
}

func (a *inventoryStockSync) SetStock(ctx context.Context, productID string, quantity int) error {
	_, err := a.svc.SetStock(ctx, &inventorymodels.SetStockRequest{
		ProductID: productID,
		Available: quantity,
	})
	return err
}

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	_ = godotenv.Load()

	if internalauth.Token() == "" {
		zap.L().Warn("INTERNAL_SERVICE_TOKEN is not set — internal-only calls to/from catalog-service will be rejected")
	}

	// --- OpenTelemetry (optional, no-op when OTEL_EXPORTER_OTLP_ENDPOINT unset) ---
	shutdownTelemetry := telemetry.Init(context.Background(), "catalog-service")
	defer shutdownTelemetry()

	cfg, err := LoadConfig()
	if err != nil {
		zap.L().Fatal("Failed to load configuration", zap.Error(err))
	}

	// --- Shared Redis (product cache + cart state + bulk-import queue) ---
	// Accepts redis:// URIs and bare host:port for .env compatibility.
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		zap.L().Warn("REDIS_URL is not a URI, using as host:port", zap.String("redis_url", cfg.RedisURL), zap.Error(err))
		redisOpts = &redis.Options{Addr: cfg.RedisURL, DB: 0}
	}
	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()

	// --- AWS ---
	awsCfg, err := awspkg.LoadAWSConfig(context.Background())
	if err != nil {
		zap.L().Fatal("Failed to load AWS config", zap.Error(err))
	}
	ddbClient := dynamodb.NewFromConfig(awsCfg)
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true // Important for LocalStack compatibility
	})
	presignClient := s3.NewPresignClient(s3Client)
	snsClient := awspkg.NewSNSClient(awsCfg)

	// --- Repositories ---
	productRepo := repository.NewDynamoAdapter(ddbClient, cfg.DDBTableProducts)
	productRepo.WithCategoryLinksTable(cfg.DDBTableLinks)
	if err := productRepo.EnsureIndexes(context.Background()); err != nil {
		zap.L().Warn("Failed to ensure product indexes", zap.Error(err))
	}
	categoryRepo := repository.NewDynamoCategoryAdapter(ddbClient, cfg.DDBTableCategories, cfg.DDBTableProducts).
		WithProductLinks(productRepo)
	inventoryRepo := inventoryrepository.NewDynamoInventoryRepository(ddbClient, cfg.DDBTableInventory)

	// --- CloudWatch (Logs + Metrics) ---
	cwLogsClient, err := awspkg.NewCloudWatchLogsClient(context.Background(), "catalog-service")
	if err != nil {
		zap.L().Warn("CloudWatch logs client init failed (non-fatal)", zap.Error(err))
	}
	_ = cwLogsClient

	metricsClient, err := awspkg.NewMetricsClient(context.Background())
	if err != nil {
		zap.L().Warn("CloudWatch metrics client init failed (non-fatal)", zap.Error(err))
	}

	// --- Services ---
	inventoryService := inventoryservices.NewInventoryService(inventoryRepo, metricsClient)
	productService := services.NewProductServiceDDB(
		productRepo,
		categoryRepo,
		s3Client,
		presignClient,
		cfg.S3Bucket,
		cfg.S3Prefix,
		cfg.AssetPublicBaseURL,
		cfg.CloudFrontDomain,
		&inventoryStockSync{svc: inventoryService},
	)
	categoryService := services.NewCategoryServiceDDB(categoryRepo, productRepo)
	productService.SetCategoryService(categoryService)

	// --- Controllers ---
	productController := controllers.NewProductController(productService, rdb)
	categoryController := controllers.NewCategoryController(categoryService, rdb)
	inventoryController := inventorycontrollers.NewInventoryController(inventoryService)
	waitingRoomService := inventoryservices.NewWaitingRoomService(rdb, logger)
	waitingRoomController := inventorycontrollers.NewWaitingRoomController(waitingRoomService)

	// Start bulk import worker (consumes persisted files from storage)
	storageDir := os.Getenv("BULK_STORAGE_DIR")
	if storageDir == "" {
		storageDir = "/tmp/bulk_imports"
	}
	services.StartBulkImportWorker(context.Background(), rdb, productService, storageDir)

	// --- HTTP router ---
	r := gin.New()
	r.Use(gin.Recovery())
	// OpenTelemetry server spans (no-op when telemetry disabled) — before all
	// other middleware so traces span the full request lifecycle.
	r.Use(telemetry.GinMiddleware("catalog-service"))
	if metricsClient != nil {
		r.Use(commonmw.MetricsMiddleware(metricsClient, "catalog-service"))
	}
	r.Use(commonmw.RequestLogger(logger))
	r.Use(func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	// Product admin writes check X-User-Role via catalog middleware.
	r.Use(commonmw.SecurityHeaders())

	// Product + category routes (paths unchanged from product-service)
	routes.RegisterRoutesLegacy(r, productController, categoryController)

	// Inventory routes (paths unchanged from inventory-service)
	inventoryroutes.RegisterRoutes(r, inventoryController, waitingRoomController)

	// Cart routes (paths unchanged from cart-service; validation is in-process)
	cartroutes.RegisterCartRoutes(r, rdb, snsClient, cfg.CartTTL, productService)

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/health/live", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/health/ready", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ready"}) })

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}

	go func() {
		zap.L().Info("Catalog Service starting", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.L().Fatal("Failed to start server", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	zap.L().Info("Shutting down Catalog Service...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		zap.L().Fatal("Server forced to shutdown", zap.Error(err))
	}

	zap.L().Info("Catalog Service stopped gracefully")
}
