package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/product-service/controllers"
)

func RegisterProductRoutes(r *gin.Engine) {
	productRoutes := r.Group("/products")
	{
		// List products with filtering, pagination, and sorting
		productRoutes.GET("/", controllers.GetProducts)
		// Get a specific product
		productRoutes.GET("/:id", controllers.GetProductByID)
		// Create a new product
		productRoutes.POST("/", controllers.CreateProduct)
		// Bulk create products
		productRoutes.POST("/bulk", controllers.CreateBulkProducts)
		// Update a product
		productRoutes.PUT("/:id", controllers.UpdateProduct)
		// Delete a product
		productRoutes.DELETE("/:id", controllers.DeleteProduct)
		// Get products by category
		productRoutes.GET("/category/:categoryId", controllers.GetProductsByCategory)
		//Get product by id for order service
		productRoutes.GET("/internal/:id", controllers.GetProductByIDInternal)
	}
}

func RegisterCategoryRoutes(r *gin.Engine) {
	categoryRoutes := r.Group("/categories")
	{
		// List all categories
		categoryRoutes.GET("/", controllers.GetCategories)
		// Get a specific category
		categoryRoutes.GET("/:id", controllers.GetCategoryByID)
		// Create a new category
		categoryRoutes.POST("/", controllers.CreateCategory)
		// POST /categories/bulk
		categoryRoutes.POST("/bulk", controllers.BulkCreateCategories)

		// Update a category
		categoryRoutes.PUT("/:id", controllers.UpdateCategory)
		// Delete a category
		categoryRoutes.DELETE("/:id", controllers.DeleteCategory)
		// Get all products in a category
		categoryRoutes.GET("/:id/products", controllers.GetCategoryProducts)
	}
}
