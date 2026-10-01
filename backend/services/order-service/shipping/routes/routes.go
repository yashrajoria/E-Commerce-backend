package routes

import (
	"order-service/middleware"
	shippingcontrollers "order-service/shipping/controllers"

	"github.com/gin-gonic/gin"
)

// RegisterShippingRoutes sets up all shipping-related routes.
func RegisterShippingRoutes(r *gin.Engine, sc *shippingcontrollers.ShippingController) {
	shipping := r.Group("/shipping")
	shipping.Use(middleware.AuthMiddleware())

	// Protected: calculate rates
	shipping.POST("/rates", sc.GetRates)
}