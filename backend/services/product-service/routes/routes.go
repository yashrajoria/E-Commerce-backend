package routes

import (
	"product-service/controllers"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// RegisterRoutes sets up all API routes
func RegisterRoutes(
	r *gin.Engine,
	productCtrl *controllers.ProductController,
	categoryCtrl *controllers.CategoryController,
	bulkHandler *controllers.BulkImportHandler,
	presignHandler *controllers.PresignedURLHandler,
) {
	registerProductRoutes(r, productCtrl, bulkHandler, presignHandler)
	registerCategoryRoutes(r, categoryCtrl)
}

// RegisterRoutesLegacy is backward compatible with old controller structure
// Use this if you haven't created the separate handlers yet
func RegisterRoutesLegacy(
	r *gin.Engine,
	productCtrl *controllers.ProductController,
	categoryCtrl *controllers.CategoryController,
) {
	// Create handlers from the main controller
	// These handlers share the same service and redis instances
	bulkHandler := controllers.NewBulkImportHandler(
		productCtrl.GetService(),
		productCtrl.GetRedis(),
		productCtrl.GetCache(),
		productCtrl.GetValidator(),
	)

	presignHandler := controllers.NewPresignedURLHandler(
		productCtrl.GetService(),
	)

	RegisterRoutes(r, productCtrl, categoryCtrl, bulkHandler, presignHandler)
}

func registerProductRoutes(
	r *gin.Engine,
	productCtrl *controllers.ProductController,
	bulkHandler *controllers.BulkImportHandler,
	presignHandler *controllers.PresignedURLHandler,
) {
	products := r.Group("/products")
	{
		// Basic CRUD operations
		products.GET("/", productCtrl.GetProducts)         // List with filters
		products.GET("/:id", productCtrl.GetProductByID)   // Get single product
		products.POST("/", productCtrl.CreateProduct)      // Create product
		products.PUT("/:id", productCtrl.UpdateProduct)    // Update product
		products.DELETE("/:id", productCtrl.DeleteProduct) // Delete product

		// Presigned URL generation for S3 uploads
		products.GET("/presign", presignHandler.GetPresignUpload)              // Legacy: presign by SKU
		products.POST("/:id/images/presign", presignHandler.PostPresignUpload) // Presign for specific product

		// Bulk import operations
		products.POST("/bulk/validate", bulkHandler.ValidateBulkImport)    // Validate CSV
		products.POST("/bulk", bulkHandler.CreateBulkProducts)             // Import CSV (sync/async)
		products.GET("/bulk/jobs/:id", bulkHandler.GetBulkImportJobStatus) // Get job status

		// Internal service-to-service endpoint
		products.GET("/internal/:id", productCtrl.GetProductByIDInternal)
	}
}

func registerCategoryRoutes(
	r *gin.Engine,
	categoryCtrl *controllers.CategoryController,
) {
	categories := r.Group("/categories")
	{
		// CRUD operations
		categories.GET("/", categoryCtrl.GetCategories)        // Get category tree
		categories.GET("/:id", categoryCtrl.GetCategory)       // Get single category
		categories.POST("/", categoryCtrl.CreateCategory)      // Create category
		categories.PUT("/:id", categoryCtrl.UpdateCategory)    // Update category
		categories.DELETE("/:id", categoryCtrl.DeleteCategory) // Delete category
	}
}

// SetupRouter creates a new Gin router with all routes registered
// This is a convenience function for easy setup
func SetupRouter(
	productService controllers.ProductServiceAPI,
	categoryService controllers.CategoryServiceAPI,
	redisClient *redis.Client,
) *gin.Engine {
	r := gin.Default()

	// Create controllers
	productCtrl := controllers.NewProductController(productService, redisClient)
	categoryCtrl := controllers.NewCategoryController(categoryService)

	// Create handlers
	bulkHandler := controllers.NewBulkImportHandler(
		productService,
		redisClient,
		productCtrl.GetCache(),
		productCtrl.GetValidator(),
	)

	presignHandler := controllers.NewPresignedURLHandler(productService)

	// Register all routes
	RegisterRoutes(r, productCtrl, categoryCtrl, bulkHandler, presignHandler)

	return r
}
