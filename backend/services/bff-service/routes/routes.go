package routes

import (
	"bff-service/controllers"
	"bff-service/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine, ctrl *controllers.BFFController) {
	r.GET("/health", ctrl.Health)
	r.GET("/health/live", ctrl.Health)
	r.GET("/health/ready", ctrl.Health)

	// Public routes - no auth required
	public := r.Group("/bff")
	{
		// Auth flows (login/register/logout/refresh/status)
		public.POST("/auth/register", ctrl.Proxy("POST", "/auth/register"))
		public.POST("/auth/login", ctrl.Proxy("POST", "/auth/login"))
		public.POST("/auth/verify-email", ctrl.Proxy("POST", "/auth/verify-email"))
		public.POST("/auth/refresh", ctrl.Proxy("POST", "/auth/refresh"))

		// Public product pages
		public.GET("/products", ctrl.Proxy("GET", "/products"))
		public.GET("/products/:id", ctrl.ProductByID)
		public.GET("/categories", ctrl.Proxy("GET", "/categories"))

		// Home page: products + categories
		public.GET("/home", ctrl.Home)
	}

	// Protected routes - require authentication
	protected := r.Group("/bff")
	protected.Use(middleware.AuthMiddleware())
	{
		// Auth flows
		protected.POST("/auth/logout", ctrl.Proxy("POST", "/auth/logout"))
		protected.GET("/auth/status", ctrl.Proxy("GET", "/auth/status"))

		// Cart page
		protected.GET("/cart", ctrl.Proxy("GET", "/cart"))
		protected.POST("/cart/add", ctrl.Proxy("POST", "/cart/add"))
		protected.DELETE("/cart/remove/:product_id", ctrl.CartRemoveItem)
		protected.DELETE("/cart/clear", ctrl.Proxy("DELETE", "/cart/clear"))
		protected.POST("/cart/checkout", ctrl.Proxy("POST", "/cart/checkout"))
		// Orchestrated checkout: creates order and checkout session, returns URL
		protected.POST("/checkout", ctrl.Checkout)

		// Orders page
		protected.GET("/orders", ctrl.Proxy("GET", "/orders"))
		protected.GET("/orders/:id", ctrl.OrderByID)

		// Profile settings
		protected.GET("/profile", ctrl.Profile)
		protected.PUT("/users/profile", ctrl.Proxy("PUT", "/users/profile"))
		protected.POST("/users/change-password", ctrl.Proxy("POST", "/users/change-password"))

		// Payments
		protected.GET("/payment/status/by-order/:order_id", ctrl.PaymentStatusByOrderID)
		protected.POST("/payment/verify-payment", ctrl.Proxy("POST", "/payment/verify-payment"))

		// Promotions / Coupons
		protected.POST("/promotions/validate", ctrl.Proxy("POST", "/coupons/validate"))
		protected.GET("/promotions/:code", ctrl.Proxy("GET", "/coupons/:code"))
	}

	// Admin-only proxy routes
	admin := r.Group("/bff/admin")
	admin.Use(middleware.AuthMiddleware(), middleware.AdminAuthMiddleware())
	{
		admin.GET("/promotions", ctrl.Proxy("GET", "/coupons"))
		admin.POST("/promotions", ctrl.Proxy("POST", "/coupons"))
		admin.DELETE("/promotions/:code", ctrl.Proxy("DELETE", "/coupons/:code"))

		admin.POST("/categories/bulk", ctrl.Proxy("POST", "/categories/bulk"))
	}
}
