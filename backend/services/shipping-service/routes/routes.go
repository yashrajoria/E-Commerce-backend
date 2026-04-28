package routes

import (
	"shipping-service/controllers"
	"shipping-service/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterShippingRoutes sets up all shipping-related routes.
func RegisterShippingRoutes(r *gin.Engine, sc *controllers.ShippingController) {
	shipping := r.Group("/shipping")
	shipping.Use(middleware.AuthMiddleware())

	// Protected: calculate rates
	shipping.POST("/rates", sc.GetRates)
}
