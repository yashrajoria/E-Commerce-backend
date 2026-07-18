package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/inventory-service/controllers"
	"github.com/yashrajoria/inventory-service/middleware"
)

// RegisterRoutes registers all inventory service routes.
// Admin mutations require X-User-Role=admin (via gateway).
// reserve/release/confirm/check require INTERNAL_SERVICE_TOKEN (order/product mesh).
func RegisterRoutes(r *gin.Engine, ctrl *controllers.InventoryController) {
	// Prevent /inventory ↔ /inventory/ 301 loops through the gateway proxy.
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false

	inventory := r.Group("/inventory")
	{
		// Admin list/create — register before /:productId
		admin := inventory.Group("")
		admin.Use(middleware.AdminOnly())
		{
			admin.GET("", ctrl.ListStock)
			admin.GET("/", ctrl.ListStock)
			admin.POST("", ctrl.SetStock)
			admin.POST("/", ctrl.SetStock)
			admin.PUT("/:productId", ctrl.UpdateStock)
		}

		inventory.GET("/:productId", ctrl.GetStock)

		mesh := inventory.Group("")
		mesh.Use(middleware.RequireInternalServiceToken())
		{
			mesh.POST("/check", ctrl.CheckStock)
			mesh.POST("/reserve", ctrl.ReserveStock)
			mesh.POST("/release", ctrl.ReleaseStock)
			mesh.POST("/confirm", ctrl.ConfirmStock)
		}
	}
}
