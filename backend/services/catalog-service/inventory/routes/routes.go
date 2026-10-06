package routes

import (
	"github.com/gin-gonic/gin"
	"catalog-service/inventory/controllers"
	"catalog-service/inventory/middleware"
)

// RegisterRoutes registers all inventory service routes.
// Admin mutations require X-User-Role=admin (via gateway).
// reserve/release/confirm/restock/check require INTERNAL_SERVICE_TOKEN (order/product mesh).
func RegisterRoutes(r *gin.Engine, ctrl *controllers.InventoryController, wsCtrl *controllers.WaitingRoomController) {
	// Prevent /inventory ↔ /inventory/ 301 loops through the gateway proxy.
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false

	inventory := r.Group("/inventory")
	{
		// Flash Sale Virtual Waiting Room
		flashSale := inventory.Group("/flash-sale")
		{
			flashSale.POST("/enter", wsCtrl.Enter)
			flashSale.GET("/status", wsCtrl.Status)
			flashSale.POST("/claim", wsCtrl.Claim)
			flashSale.POST("/release", wsCtrl.Release)

			adminFlash := flashSale.Group("")
			adminFlash.Use(middleware.AdminOnly())
			{
				adminFlash.POST("/configure", wsCtrl.Configure)
				adminFlash.POST("/reset", wsCtrl.Reset)
			}
		}

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
			mesh.POST("/restock", ctrl.RestockStock)
		}
	}
}

