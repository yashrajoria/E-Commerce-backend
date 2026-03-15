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
	notifications := forwardTo("http://notification-service:8092/notifications") // fixed: was 8089

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
	// PUBLIC ROUTES — no authentication required
	// =========================================================================

	// Docs
	public.GET("/docs", forwardTo("http://bff-service:8088/docs"))
	public.GET("/docs/*any", forwardTo("http://bff-service:8088/docs"))

	// Stripe webhook — Stripe calls this directly, no auth
	public.POST("/stripe/webhook", forwardTo("http://payment-service:8087/stripe/webhook"))

	// Auth — public actions only
	public.POST("/auth/login", authProxy)
	public.POST("/auth/register", authProxy)
	public.POST("/auth/verify-email", authProxy)
	public.POST("/auth/resend-verification", authProxy)

	// Products — read only, public browsing
	public.GET("/products", products)
	public.GET("/products/*any", products)

	// Categories — read only, public browsing
	public.GET("/categories", categories)
	public.GET("/categories/*any", categories)

	// BFF — all methods public, BFF handles its own internal auth
	public.GET("/bff", bff)
	public.GET("/bff/*any", bff)
	public.POST("/bff", bff)
	public.POST("/bff/*any", bff)
	public.PUT("/bff/*any", bff)
	public.DELETE("/bff/*any", bff)

	// =========================================================================
	// PROTECTED ROUTES — JWT required
	// =========================================================================

	// Auth — protected actions
	protected.POST("/auth/logout", authProxy)
	protected.POST("/auth/refresh", authProxy)
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

	// Orders — create and read
	protected.GET("/orders", orders)
	protected.GET("/orders/*any", orders)
	protected.POST("/orders", orders)
	protected.POST("/orders/*any", orders)

	// Payment
	protected.POST("/payment", payment)
	protected.POST("/payment/*any", payment)
	protected.GET("/payment/*any", payment)

	// Inventory — read
	protected.GET("/inventory/:productId", inventory)
	protected.POST("/inventory/check", inventory)

	// Coupons — validate and read single
	protected.POST("/coupons/validate", coupons)
	protected.GET("/coupons/:code", coupons)

	// Shipping
	protected.POST("/shipping/rates", shipping)
	protected.POST("/shipping/labels", shipping)
	protected.GET("/shipping/track/:tracking_code", shipping)

	// =========================================================================
	// ADMIN ROUTES — JWT + admin role required
	// =========================================================================

	// Products — write
	admin.POST("/products", products)
	admin.POST("/products/*any", products)
	admin.PUT("/products/*any", products)
	admin.DELETE("/products/*any", products)

	// Categories — write
	admin.POST("/categories", categories)
	admin.POST("/categories/*any", categories)
	admin.PUT("/categories/*any", categories)
	admin.DELETE("/categories/*any", categories)

	// Orders — write
	admin.PUT("/orders/*any", orders)
	admin.DELETE("/orders/*any", orders)

	// Inventory — write
	admin.POST("/inventory", inventory)
	admin.PUT("/inventory/:productId", inventory)

	// Coupons — write
	admin.POST("/coupons", coupons)
	admin.GET("/coupons", coupons)
	admin.DELETE("/coupons/:code", coupons)

	// Notifications — admin read
	admin.GET("/notifications/log", notifications)
}
