// Package routes registers HTTP endpoints for order placement, tracking, and cancellation.
package routes

import (
	"order-service/controllers"
	"order-service/credit"
	"order-service/middleware"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/common/internalauth"
)

// RegisterOrderRoutes configures and mounts HTTP route handlers on the router.
func RegisterOrderRoutes(r *gin.Engine, controllers *controllers.OrderController, creditControllers ...*credit.Controller) {
	orderRoutes := r.Group("/orders")
	orderRoutes.Use(middleware.AuthMiddleware())
	if len(creditControllers) > 0 && creditControllers[0] != nil {
		// Store credit endpoints trust gateway-injected X-User-* and X-Store-ID headers;
		// require mesh authentication before accepting those claims to prevent perimeter bypass.
		creditRoutes := r.Group("/store-credit")
		creditRoutes.Use(internalauth.Require(), middleware.AuthMiddleware())
		creditRoutes.GET("/balance", creditControllers[0].Balance)
		creditRoutes.GET("/ledger", creditControllers[0].Ledger)
		creditRoutes.POST("/grants", creditControllers[0].Grant)
		creditRoutes.POST("/adjustments", creditControllers[0].Adjust)
		creditRoutes.POST("/hold", creditControllers[0].Hold)
		creditRoutes.POST("/release", creditControllers[0].Release)
		creditRoutes.GET("/reconcile", creditControllers[0].Reconcile)
	}

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
