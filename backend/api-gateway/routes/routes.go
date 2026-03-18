package routes

import (
	"net/http"
	"strings"

	"api-gateway/middlewares"
	"api-gateway/utils"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

func RegisterAllRoutes(r *gin.Engine, redisClient *redis.Client) {

	// ── helper defined FIRST before any use ──────────────────────────────────
	forwardTo := func(targetBase string) gin.HandlerFunc {
		return func(c *gin.Context) {
			// SECURITY: Block public access to internal-only service endpoints
			if strings.Contains(c.Request.URL.Path, "/internal/") {
				c.JSON(http.StatusForbidden, gin.H{"error": "access to internal endpoints is restricted"})
				c.Abort()
				return
			}
			utils.ForwardRequest(c, utils.ForwardOptions{
				TargetBase: targetBase,
			})
		}
	}

	// ── Global Middlewares ────────────────────────────────────────────────────
	r.Use(middlewares.CorrelationIDMiddleware())

	// ── service targets ───────────────────────────────────────────────────────
	bff := forwardTo("http://bff-service:8088/bff/admin")
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

	// ── Global Rate Limiter ───────────────────────────────────────────────────
	if redisClient != nil {
		r.Use(middlewares.GlobalRateLimiter(redisClient))
	}

	// =========================================================================
	// PUBLIC ROUTES — no authentication required
	// =========================================================================

	// Docs
	public.GET("/docs", forwardTo("http://bff-service:8088/docs"))
	public.GET("/docs/*any", forwardTo("http://bff-service:8088/docs"))

	// Stripe webhook — Stripe calls this directly, no auth
	public.POST("/stripe/webhook", forwardTo("http://payment-service:8087/stripe/webhook"))

	// Auth — sensitive public actions (strict rate limiting)
	authStrict := public.Group("/auth")
	if redisClient != nil {
		authStrict.Use(middlewares.StrictRateLimiter(redisClient))
	}
	authStrict.POST("/login", authProxy)
	authStrict.POST("/register", authProxy)
	authStrict.POST("/resend-verification", authProxy)

	// Auth — other public actions (normal global limits)
	public.POST("/auth/verify-email", authProxy)

	// Products — read only, public browsing
	public.GET("/products", products)
	public.GET("/products/*any", products)

	// Categories — read only, public browsing
	public.GET("/categories", categories)
	public.GET("/categories/*any", categories)

	// BFF — PUBLIC (no auth required)
	bffPublic := public.Group("/bff")
	bffPublic.POST("/auth/register", bff)
	bffPublic.POST("/auth/login", bff)
	bffPublic.POST("/auth/verify-email", bff)
	bffPublic.POST("/auth/refresh", bff)
	bffPublic.GET("/products", bff)
	bffPublic.GET("/products/*any", bff)
	bffPublic.GET("/categories", bff)
	bffPublic.GET("/categories/*any", bff)
	bffPublic.GET("/home", bff)

	// BFF — PROTECTED (JWT required)
	bffProtected := protected.Group("/bff")
	bffProtected.Use(middlewares.JWTMiddleware())
	bffProtected.POST("/auth/logout", bff)
	bffProtected.GET("/auth/status", bff)
	bffProtected.GET("/cart", bff)
	bffProtected.POST("/cart/*any", bff)
	bffProtected.DELETE("/cart/*any", bff)
	bffProtected.POST("/checkout", bff)
	bffProtected.GET("/orders", bff)
	bffProtected.GET("/orders/*any", bff)
	bffProtected.GET("/profile", bff)
	bffProtected.PUT("/users/profile", bff)
	bffProtected.POST("/users/change-password", bff)
	bffProtected.GET("/payment/*any", bff)
	bffProtected.POST("/payment/*any", bff)

	// BFF — ADMIN (JWT + Admin Role required)
	bffAdmin := protected.Group("/bff/admin")
	bffAdmin.Use(middlewares.JWTMiddleware())
	bffAdmin.Use(middlewares.AdminRoleMiddleware())
	bffAdmin.Any("", bff)
	bffAdmin.Any("/*any", bff)

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
	admin.GET("/inventory", inventory)
	admin.POST("/inventory", inventory)
	admin.PUT("/inventory/:productId", inventory)

	// Coupons — write
	admin.POST("/coupons", coupons)
	admin.GET("/coupons", coupons)
	admin.DELETE("/coupons/:code", coupons)

	// Notifications — admin read
	admin.GET("/notifications/log", notifications)
}
