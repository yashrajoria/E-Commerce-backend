package routes

import (
	"promotion-service/controllers"
	"promotion-service/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterCouponRoutes sets up all coupon-related routes.
func RegisterCouponRoutes(r *gin.Engine, cc *controllers.CouponController) {
	couponRoutes := r.Group("/coupons")

	// Guest-friendly: validate only needs code + cart_total (rate-limit at gateway).
	couponRoutes.POST("/validate", cc.ValidateCoupon)

	// Authenticated reads
	authed := couponRoutes.Group("")
	authed.Use(middleware.AuthMiddleware())
	authed.GET("/:code", cc.GetCoupon)

	// Admin-only mutations / list
	adminRoutes := couponRoutes.Group("")
	adminRoutes.Use(middleware.AuthMiddleware(), middleware.AdminOnly())
	adminRoutes.POST("", cc.CreateCoupon)
	adminRoutes.GET("", cc.ListCoupons)
	adminRoutes.DELETE("/:code", cc.DeactivateCoupon)
}
