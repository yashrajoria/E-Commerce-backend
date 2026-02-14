package routes

import (
	"product-service/controllers"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine, productController *controllers.ProductController, categoryController *controllers.CategoryController) {
	productRoutes := r.Group("/products")
	{
		// List products with filtering, pagination, and sorting
		productRoutes.GET("/", productController.GetProducts)
		// Get a specific product
		productRoutes.GET("/:id", productController.GetProductByID)
		// Create a new product
		productRoutes.POST("/", productController.CreateProduct)
		// Generate a presigned upload URL for S3 (legacy GET)
		productRoutes.GET("/presign", productController.GetPresignUpload)
		// New: presign upload for a specific product id (server-side presign)
		productRoutes.POST(":id/images/presign", productController.PostPresignUpload)
		// Bulk create products
		productRoutes.POST("/bulk/validate", productController.ValidateBulkImport)

		productRoutes.POST("/bulk", productController.CreateBulkProducts)
		// Update a product
		productRoutes.PUT("/:id", productController.UpdateProduct)
		// Delete a product
		productRoutes.DELETE("/:id", productController.DeleteProduct)
		// Get products by category
		//Get product by id for order service
		productRoutes.GET("/internal/:id", productController.GetProductByIDInternal)
	}
	categoryRoutes := r.Group("/categories")
	{
		// List all categories
		categoryRoutes.GET("/", categoryController.GetCategories)
		// Get a specific category
		// categoryRoutes.GET("/:id", categoryController.GetCategoryByID)
		// Create a new category
		categoryRoutes.POST("/", categoryController.CreateCategory)
		// POST /categories/bulk
		// categoryRoutes.POST("/bulk", categoryController.BulkCreateCategories)

		// Update a category
		categoryRoutes.PUT("/:id", categoryController.UpdateCategory)
		// Delete a category
		categoryRoutes.DELETE("/:id", categoryController.DeleteCategory)
		// Get all products in a category
		// categoryRoutes.GET("/:id/products", categoryController.GetCategoryProducts)
	}
}
