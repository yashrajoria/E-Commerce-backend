package routes

import (
	"order-service/controllers"
	"order-service/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterOrderRoutes(r *gin.Engine) {
	orderRoutes := r.Group("/orders")
	orderRoutes.Use(middleware.AuthMiddleware())
	orderRoutes.GET("/", controllers.GetOrders) // User's own orders
	// orderRoutes.POST("/", controllers.CreateOrder)    // Create a new order
	orderRoutes.GET("/:id", controllers.GetOrderByID) // Get order by ID

	adminRoutes := r.Group("/admin")
	// adminRoutes.Use(middleware.AuthMiddleware(), middleware.AdminOnly()) // Admin middleware to restrict access
	adminRoutes.GET("/orders", controllers.GetAllOrders) // All orders
}
