package routes

import (
	"order-service/controllers"
	"order-service/middleware"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/common/internalauth"
)

func RegisterOrderRoutes(r *gin.Engine, controllers *controllers.OrderController) {
	orderRoutes := r.Group("/orders")
	orderRoutes.Use(middleware.AuthMiddleware())

	// User routes
	orderRoutes.GET("/", controllers.GetOrders)
	orderRoutes.GET("/:id", controllers.GetOrderByID)
	orderRoutes.PUT("/:id/cancel", middleware.AdminOnly(), controllers.CancelOrder)
	// Refunds trust gateway-injected X-User-* claims, so require the mesh
	// credential before accepting those claims. This prevents direct callers
	// from forging an admin identity at the order-service listener.
	orderRoutes.POST("/:id/refund", internalauth.Require(), controllers.RefundOrder)

	// Admin-only routes
	adminRoutes := orderRoutes.Group("/admin")
	adminRoutes.Use(middleware.AdminOnly())
	adminRoutes.GET("/", controllers.GetAllOrders)
	adminRoutes.GET("/stats", controllers.GetRevenueStats)
}
