package routes

import (
	"api-gateway/middlewares"
	"api-gateway/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

func RegisterAllRoutes(r *gin.Engine) {

	// ── helper defined FIRST before any use ──────────────────────────────────
	forwardTo := func(targetBase string) gin.HandlerFunc {
		return func(c *gin.Context) {
			utils.ForwardRequest(c, utils.ForwardOptions{
				TargetBase: targetBase,
			})
		}
	}

	// ── service targets ───────────────────────────────────────────────────────
	bff := forwardTo("http://bff-service:8088/bff")
	products := forwardTo("http://product-service:8082/products")
	categories := forwardTo("http://product-service:8082/categories")
	users := forwardTo("http://user-service:8085/users")
	cart := forwardTo("http://cart-service:8086/cart")
	orders := forwardTo("http://order-service:8083/orders")
	payment := forwardTo("http://payment-service:8087/payment")
	inventory := forwardTo("http://inventory-service:8084/inventory")
	coupons := forwardTo("http://promotion-service:8090/coupons")
	shipping := forwardTo("http://shipping-service:8091/shipping")
	authProxy := forwardTo("http://auth-service:8081/auth")
	notifications := forwardTo("http://notification-service:8089/notifications")

	// ── groups ────────────────────────────────────────────────────────────────
	public := r.Group("/")
	protected := r.Group("/")
	protected.Use(middlewares.JWTMiddleware())
	admin := protected.Group("/")
	admin.Use(middlewares.AdminRoleMiddleware())

	// ── health ────────────────────────────────────────────────────────────────
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "OK", "service": "api-gateway"})
	})

	// =========================================================================
	// PUBLIC ROUTES
	// =========================================================================

	// Docs
	public.GET("/docs", forwardTo("http://bff-service:8088/docs"))
	public.GET("/docs/*any", forwardTo("http://bff-service:8088/docs"))

	// Products (read only — public)
	public.GET("/products", products)
	public.GET("/products/*any", products)

	// Categories (read only — public)
	public.GET("/categories", categories)
	public.GET("/categories/*any", categories)

	// BFF (public GET only — e.g. product listing pages)
	public.GET("/bff", bff)
	public.GET("/bff/home", bff)
	public.GET("/bff/products", bff)
	public.GET("/bff/products/*any", bff)
	public.GET("/bff/categories", bff)

	// BFF auth (public)
	public.POST("/bff/auth/register", bff)
	public.POST("/bff/auth/login", bff)
	public.POST("/bff/auth/verify-email", bff)
	public.POST("/bff/auth/refresh", bff)

	// Auth (public — login, register, verify)
	auth := r.Group("/auth")
	auth.POST("/*any", authProxy)

	// Stripe webhook (public — Stripe calls this directly)
	public.POST("/stripe/webhook", forwardTo("http://payment-service:8087/stripe/webhook"))

	// =========================================================================
	// PROTECTED ROUTES (JWT required)
	// =========================================================================

	// Auth (protected GET — e.g. /auth/me, refresh)
	protected.GET("/auth/*any", authProxy)

	// Users
	protected.GET("/users", users)
	protected.GET("/users/*any", users)
	protected.POST("/users/*any", users)
	protected.PUT("/users/*any", users)
	protected.DELETE("/users/*any", users)

	// Cart
	protected.GET("/cart", cart)
	protected.GET("/cart/*any", cart)
	protected.POST("/cart/*any", cart)
	protected.PUT("/cart/*any", cart)
	protected.DELETE("/cart/*any", cart)

	// Orders (protected read + create)
	protected.GET("/orders", orders)
	protected.GET("/orders/*any", orders)
	protected.POST("/orders", orders)
	protected.POST("/orders/*any", orders)

	// Payment
	protected.POST("/payment", payment)
	protected.POST("/payment/*any", payment)
	protected.GET("/payment/*any", payment)

	// Inventory (protected read)
	protected.GET("/inventory/:productId", inventory)
	protected.POST("/inventory/check", inventory)

	// Coupons (protected read + validate)
	protected.POST("/coupons/validate", coupons)
	protected.GET("/coupons/:code", coupons)

	// Shipping (protected)
	protected.POST("/shipping/rates", shipping)
	protected.POST("/shipping/labels", shipping)
	protected.GET("/shipping/track/:tracking_code", shipping)

	// BFF (protected POST + GET for authenticated pages)
	protected.POST("/bff/auth/logout", bff)
	protected.POST("/bff/cart/add", bff)
	protected.POST("/bff/cart/checkout", bff)
	protected.POST("/bff/checkout", bff)
	protected.POST("/bff/users/change-password", bff)
	protected.POST("/bff/payment/verify-payment", bff)
	protected.GET("/bff/auth/status", bff)
	protected.GET("/bff/cart", bff)
	protected.GET("/bff/orders", bff)
	protected.GET("/bff/orders/:id", bff)
	protected.GET("/bff/profile", bff)
	protected.GET("/bff/payment/status/by-order/:order_id", bff)

	// =========================================================================
	// ADMIN ROUTES (JWT + admin role required)
	// =========================================================================

	// Products (admin write)
	admin.POST("/products", products)
	admin.POST("/products/*any", products)
	admin.PUT("/products/*any", products)
	admin.DELETE("/products/*any", products)

	// Categories (admin write)
	admin.POST("/categories", categories)
	admin.POST("/categories/*any", categories)
	admin.PUT("/categories/*any", categories)
	admin.DELETE("/categories/*any", categories)

	// Orders (admin write)
	admin.PUT("/orders/*any", orders)
	admin.DELETE("/orders/*any", orders)

	// Inventory (admin write)
	admin.POST("/inventory", inventory)
	admin.PUT("/inventory/:productId", inventory)

	// Coupons (admin write)
	admin.POST("/coupons", coupons)
	admin.GET("/coupons", coupons)
	admin.DELETE("/coupons/:code", coupons)

	// Notifications (admin read — log viewer)
	admin.GET("/notifications/log", notifications)
}
