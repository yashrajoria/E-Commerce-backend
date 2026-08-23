package routes

import (
	"product-service/controllers"
	"product-service/middleware"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/yashrajoria/common/internalauth"
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
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false

	products := r.Group("/products")
	{
		// Public reads — both "" and "/" so gateway /products does not 301-loop
		products.GET("", productCtrl.GetProducts)
		products.GET("/", productCtrl.GetProducts)

		// Admin writes / helpers (before /:id so "presign" is not captured as an id)
		admin := products.Group("/")
		admin.Use(middleware.AdminOnly())
		{
			admin.POST("/", productCtrl.CreateProduct)
			admin.GET("/presign", presignHandler.GetPresignUpload)
			admin.POST("/bulk/validate", bulkHandler.ValidateBulkImport)
			admin.POST("/bulk", bulkHandler.CreateBulkProducts)
			admin.GET("/bulk/jobs/:id", bulkHandler.GetBulkImportJobStatus)
			admin.POST("/bulk/delete", productCtrl.BulkDeleteProducts)
			admin.PUT("/:id", productCtrl.UpdateProduct)
			admin.DELETE("/:id", productCtrl.DeleteProduct)
			admin.POST("/:id/images/presign", presignHandler.PostPresignUpload)
		}

		products.GET("/:id", productCtrl.GetProductByID)
		products.GET(
			"/internal/:id",
			internalauth.Require(),
			productCtrl.GetProductByIDInternal,
		)
		products.POST(
			"/internal/batch-validate",
			internalauth.Require(),
			productCtrl.BatchValidateInternal,
		)
	}
}

func registerCategoryRoutes(
	r *gin.Engine,
	categoryCtrl *controllers.CategoryController,
) {
	categories := r.Group("/categories")
	{
		categories.GET("", categoryCtrl.GetCategories)
		categories.GET("/", categoryCtrl.GetCategories)
		categories.GET("/:id", categoryCtrl.GetCategory)

		admin := categories.Group("/")
		admin.Use(middleware.AdminOnly())
		{
			admin.POST("/", categoryCtrl.CreateCategory)
			admin.PUT("/:id", categoryCtrl.UpdateCategory)
			admin.DELETE("/:id", categoryCtrl.DeleteCategory)
			admin.POST("/bulk", categoryCtrl.CreateBulkCategories)
		}
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
	categoryCtrl := controllers.NewCategoryController(categoryService, redisClient)

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
