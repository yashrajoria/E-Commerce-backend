package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/inventory-service/controllers"
)

// RegisterRoutes registers all inventory service routes
func RegisterRoutes(r *gin.Engine, ctrl *controllers.InventoryController) {
	inventory := r.Group("/inventory")
	{
		// Public/internal endpoints
		inventory.GET("/:productId", ctrl.GetStock)
		inventory.POST("/check", ctrl.CheckStock)

		// Admin/internal endpoints for stock management
		inventory.POST("", ctrl.SetStock)
		inventory.PUT("/:productId", ctrl.UpdateStock)

		// Internal endpoints used by order service
		inventory.POST("/reserve", ctrl.ReserveStock)
		inventory.POST("/release", ctrl.ReleaseStock)
		inventory.POST("/confirm", ctrl.ConfirmStock)
	}
}
