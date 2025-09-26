package routes

import (
	"order-service/controllers"
	"order-service/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterOrderRoutes(r *gin.Engine) {
	orderRoutes := r.Group("/orders")
	orderRoutes.Use(middleware.AuthMiddleware())

	// User routes
	orderRoutes.GET("/", controllers.GetOrders)
	orderRoutes.GET("/:id", controllers.GetOrderByID)

	// Admin-only routes
	adminRoutes := orderRoutes.Group("/admin")
	adminRoutes.Use(middleware.AdminOnly())
	adminRoutes.GET("/", controllers.GetAllOrders)
}
