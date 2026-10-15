// Package routes registers HTTP endpoints for store administration and tenant features.
package routes

import (
	"identity-service/middleware"
	"identity-service/stores/controllers"

	"github.com/gin-gonic/gin"
)

// RegisterStoreRoutes configures and mounts HTTP route handlers on the router.
func RegisterStoreRoutes(public *gin.RouterGroup, protected *gin.RouterGroup, ctrl *controllers.StoreController) {
	// Public store lookup by slug and plans listing
	public.GET("/stores/by-slug/:slug", ctrl.GetStoreBySlug)
	public.GET("/plans", ctrl.ListPlans)

	// Authenticated store operations
	stores := protected.Group("/stores")
	stores.Use(middleware.AuthMiddleware())
	{
		stores.POST("", ctrl.CreateStore)
		stores.GET("", ctrl.ListStores)
		stores.GET("/:id", ctrl.GetStore)
		stores.GET("/:id/membership/me", ctrl.GetMyMembership)
		stores.PUT("/:id", ctrl.UpdateStore)
		stores.DELETE("/:id", ctrl.DeleteStore)

		// Member management
		stores.GET("/:id/members", ctrl.ListMembers)
		stores.POST("/:id/invite", ctrl.InviteMember)
		stores.PUT("/:id/members/:userId", ctrl.UpdateMemberRole)
		stores.DELETE("/:id/members/:userId", ctrl.RemoveMember)

		// Settings & Subscription
		stores.GET("/:id/settings", ctrl.GetSettings)
		stores.PUT("/:id/settings", ctrl.UpdateSettings)
		stores.GET("/:id/subscription", ctrl.GetSubscription)
	}
}
