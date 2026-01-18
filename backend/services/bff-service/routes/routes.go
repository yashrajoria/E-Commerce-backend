package routes

import (
	"bff-service/controllers"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine, ctrl *controllers.BFFController) {
	r.GET("/health", ctrl.Health)

	bff := r.Group("/bff")
	{
		// Home page: products + categories
		bff.GET("/home", ctrl.Home)
		// Profile page: profile + orders
		bff.GET("/profile", ctrl.Profile)

		// Auth flows (login/register/logout/refresh/status)
		bff.POST("/auth/register", ctrl.Proxy("POST", "/auth/register"))
		bff.POST("/auth/login", ctrl.Proxy("POST", "/auth/login"))
		bff.POST("/auth/logout", ctrl.Proxy("POST", "/auth/logout"))
		bff.POST("/auth/refresh", ctrl.Proxy("POST", "/auth/refresh"))
		bff.GET("/auth/status", ctrl.Proxy("GET", "/auth/status"))
		bff.POST("/auth/verify-email", ctrl.Proxy("POST", "/auth/verify-email"))

		// Products pages
		bff.GET("/products", ctrl.Proxy("GET", "/products"))
		bff.GET("/products/:id", ctrl.ProductByID)
		bff.GET("/categories", ctrl.Proxy("GET", "/categories"))

		// Cart page
		bff.GET("/cart", ctrl.Proxy("GET", "/cart"))
		bff.POST("/cart/add", ctrl.Proxy("POST", "/cart/add"))
		bff.DELETE("/cart/remove/:product_id", ctrl.CartRemoveItem)
		bff.DELETE("/cart/clear", ctrl.Proxy("DELETE", "/cart/clear"))
		bff.POST("/cart/checkout", ctrl.Proxy("POST", "/cart/checkout"))

		// Orders page
		bff.GET("/orders", ctrl.Proxy("GET", "/orders"))
		bff.GET("/orders/:id", ctrl.OrderByID)

		// Profile settings
		bff.PUT("/users/profile", ctrl.Proxy("PUT", "/users/profile"))
		bff.POST("/users/change-password", ctrl.Proxy("POST", "/users/change-password"))

		// Payments
		bff.GET("/payment/status/by-order/:order_id", ctrl.PaymentStatusByOrderID)
		bff.POST("/payment/verify-payment", ctrl.Proxy("POST", "/payment/verify-payment"))
	}
}
