package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/product-service/controllers"
)

func RegisterProductRoutes(r *gin.Engine) {
	productRoutes := r.Group("/products")
	{
		productRoutes.GET("/", controllers.GetProducts)
		productRoutes.GET("/:id", controllers.GetProductByID)
		productRoutes.POST("/", controllers.CreateProduct)
		productRoutes.PUT("/:id", controllers.UpdateProduct)
		productRoutes.DELETE("/:id", controllers.DeleteProduct)
	}
}

func RegisterCategoryRoutes(r *gin.Engine) {
	categoryRoutes := r.Group("/category")
	{
		categoryRoutes.GET("/", controllers.GetCategories)
		// categoryRoutes.GET("/:id", controllers.GetCategoryByID)
		categoryRoutes.POST("/", controllers.CreateCategory)
		// categoryRoutes.PUT("/:id", controllers.UpdateCategory)
		// categoryRoutes.DELETE("/:id", controllers.DeleteCategory)
	}
}
