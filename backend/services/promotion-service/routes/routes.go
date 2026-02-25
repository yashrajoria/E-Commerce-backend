package routes

import (
	"promotion-service/controllers"
	"promotion-service/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterCouponRoutes sets up all coupon-related routes.
func RegisterCouponRoutes(r *gin.Engine, cc *controllers.CouponController) {
	couponRoutes := r.Group("/coupons")

	// Public / internal routes (protected by auth middleware)
	couponRoutes.Use(middleware.AuthMiddleware())
	couponRoutes.POST("/validate", cc.ValidateCoupon)
	couponRoutes.GET("/:code", cc.GetCoupon)

	// Admin-only routes
	adminRoutes := couponRoutes.Group("")
	adminRoutes.Use(middleware.AdminOnly())
	adminRoutes.POST("", cc.CreateCoupon)
	adminRoutes.GET("", cc.ListCoupons)
	adminRoutes.DELETE("/:code", cc.DeactivateCoupon)
}
