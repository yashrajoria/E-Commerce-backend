package routes

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"api-gateway/middlewares"
	"api-gateway/utils"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// getEnv returns the env var value for key, or fallback if unset/blank.
func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// newAdminGroup creates a sub-group of parent gated by JWT (inherited) + the
// admin role, in one place, so every admin-only route group is guaranteed to
// carry AdminRoleMiddleware by construction rather than relying on each call
// site remembering to attach it.
func newAdminGroup(parent *gin.RouterGroup, relativePath string) *gin.RouterGroup {
	g := parent.Group(relativePath)
	g.Use(middlewares.AdminRoleMiddleware())
	return g
}

func RegisterAllRoutes(r *gin.Engine, redisClient *redis.Client) {

	// ── Global Middlewares ────────────────────────────────────────────────────
	r.Use(middlewares.RequestIDMiddleware())
	if redisClient != nil {
		r.Use(middlewares.GlobalRateLimiter(redisClient))
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "api-gateway"})
	})
	r.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "api-gateway"})
	})
	r.GET("/health/ready", func(c *gin.Context) {
		if redisClient != nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			defer cancel()
			if err := redisClient.Ping(ctx).Err(); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "error": err.Error()})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready", "service": "api-gateway"})
	})

	// ── forwardTo helper ──────────────────────────────────────────────────────
	// URL path so it can append it correctly to TargetBase.
	// with an empty path (""), producing 404s from bff-service.
	forwardTo := func(targetBase string) gin.HandlerFunc {
		return func(c *gin.Context) {
			// SECURITY: Block public access to internal-only service endpoints.
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

	// ── service base URLs ─────────────────────────────────────────────────────
	// Overridable per-environment via env vars; default to the Docker Compose
	// service names/ports used in local/LocalStack deployments.
	bffBase := getEnv("BFF_SERVICE_URL", "http://bff-service:8088")
	productBase := getEnv("PRODUCT_SERVICE_URL", "http://product-service:8082")
	userBase := getEnv("USER_SERVICE_URL", "http://user-service:8085")
	cartBase := getEnv("CART_SERVICE_URL", "http://cart-service:8086")
	orderBase := getEnv("ORDER_SERVICE_URL", "http://order-service:8083")
	paymentBase := getEnv("PAYMENT_SERVICE_URL", "http://payment-service:8087")
	inventoryBase := getEnv("INVENTORY_SERVICE_URL", "http://inventory-service:8084")
	promotionBase := getEnv("PROMOTION_SERVICE_URL", "http://promotion-service:8090")
	shippingBase := getEnv("SHIPPING_SERVICE_URL", "http://shipping-service:8091")
	authBase := getEnv("AUTH_SERVICE_URL", "http://auth-service:8081")
	notificationBase := getEnv("NOTIFICATION_SERVICE_URL", "http://notification-service:8092")
	agentBase := getEnv("AGENT_SERVICE_URL", "http://agent-service:8000")

	// ── service targets ───────────────────────────────────────────────────────
	bff := forwardTo(bffBase + "/bff")
	products := forwardTo(productBase + "/products")
	categories := forwardTo(productBase + "/categories")
	users := forwardTo(userBase + "/users")
	cart := forwardTo(cartBase + "/cart")
	orders := forwardTo(orderBase + "/orders")
	payment := forwardTo(paymentBase + "/payment")
	inventory := forwardTo(inventoryBase + "/inventory")
	coupons := forwardTo(promotionBase + "/coupons")
	shipping := forwardTo(shippingBase + "/shipping")
	authProxy := forwardTo(authBase + "/auth")
	notifications := forwardTo(notificationBase + "/notifications")
	agent := forwardTo(agentBase + "/agent")

	// ── route groups ──────────────────────────────────────────────────────────
	public := r.Group("/")
	protected := r.Group("/")
	protected.Use(middlewares.JWTMiddleware())
	admin := newAdminGroup(protected, "/")

	// =========================================================================
	// PUBLIC ROUTES — no authentication required
	// =========================================================================

	// Docs
	public.GET("/docs", forwardTo(bffBase+"/docs"))
	public.GET("/docs/*any", forwardTo(bffBase+"/docs"))

	// Stripe webhook — Stripe calls this directly, no auth
	public.POST("/stripe/webhook", forwardTo(paymentBase+"/stripe/webhook"))

	// Auth — sensitive public actions (strict rate limiting)
	authStrict := public.Group("/auth")
	if redisClient != nil {
		authStrict.Use(middlewares.StrictRateLimiter(redisClient))
	}
	authStrict.POST("/login", authProxy)
	authStrict.POST("/register", authProxy)
	authStrict.POST("/resend-verification", authProxy)
	// Refresh must be public: access JWT is often expired when this is called.
	// Auth is the refresh_token cookie, not __session.
	authStrict.POST("/refresh", authProxy)

	// Auth — other public actions
	public.POST("/auth/verify-email", authProxy)

	// Products — read only, public browsing
	public.GET("/products", products)
	public.GET("/products/*any", products)

	// Categories — read only, public browsing
	public.GET("/categories", categories)
	public.GET("/categories/*any", categories)

	// Coupons — guest checkout can validate without login
	public.POST("/coupons/validate", coupons)

	// ── BFF routes ────────────────────────────────────────────────────────────

	// BFF — PUBLIC (no auth required)
	bffPublic := public.Group("/bff")
	bffPublic.POST("/auth/register", bff)
	bffPublic.POST("/auth/login", bff)
	bffPublic.POST("/auth/verify-email", bff)
	bffPublic.POST("/auth/resend-verification", bff)
	bffPublic.POST("/auth/refresh", bff)
	bffPublic.POST("/promotions/validate", bff)
	bffPublic.GET("/products", bff)
	bffPublic.GET("/products/*action", bff)
	bffPublic.GET("/categories", bff)
	bffPublic.GET("/categories/*action", bff)
	bffPublic.GET("/home", bff)

	// BFF — PROTECTED (JWT required)
	bffProtected := protected.Group("/bff")
	bffProtected.POST("/auth/logout", bff)
	bffProtected.GET("/auth/status", bff)
	bffProtected.GET("/cart", bff)
	bffProtected.POST("/cart/*action", bff)
	bffProtected.DELETE("/cart/*action", bff)
	bffProtected.POST("/checkout", bff)
	bffProtected.GET("/orders", bff)
	bffProtected.GET("/orders/*action", bff)
	bffProtected.GET("/profile", bff)
	bffProtected.PUT("/users/profile", bff)
	bffProtected.POST("/users/change-password", bff)
	bffProtected.GET("/payment/*action", bff)
	bffProtected.POST("/payment/*action", bff)

	// BFF — ADMIN (JWT + Admin Role required)
	bffAdmin := newAdminGroup(protected, "/bff/admin")

	// Keep dashboard and analytics routed to BFF
	bffAdmin.GET("/dashboard", bff)
	bffAdmin.GET("/reports/*any", bff)

	// Map CRUD operations directly to microservices to avoid bff double-proxy
	bffAdmin.GET("/products", products)
	bffAdmin.POST("/products", products)
	bffAdmin.GET("/products/presign", forwardTo(productBase+"/products/presign"))
	bffAdmin.PUT("/products/*any", products)
	bffAdmin.POST("/products/*any", products)
	bffAdmin.DELETE("/products/*any", products)

	bffAdmin.GET("/categories", categories)
	bffAdmin.POST("/categories", categories)
	bffAdmin.PUT("/categories/*any", categories)
	bffAdmin.DELETE("/categories/*any", categories)

	bffAdmin.GET("/users", users)
	bffAdmin.POST("/users", forwardTo(authBase+"/auth/admin/users"))
	bffAdmin.PUT("/users/*any", users)
	bffAdmin.DELETE("/users/*any", users)

	bffAdmin.GET("/orders", forwardTo(orderBase+"/orders/admin/"))
	bffAdmin.GET("/orders/*any", orders)
	bffAdmin.PUT("/orders/*any", orders)

	bffAdmin.GET("/inventory", inventory)
	bffAdmin.PUT("/inventory/*any", inventory)

	bffAdmin.GET("/coupons", coupons)
	bffAdmin.POST("/coupons", coupons)
	bffAdmin.PUT("/coupons/*any", coupons)
	bffAdmin.DELETE("/coupons/*any", coupons)

	bffAdmin.GET("/notifications", notifications)
	bffAdmin.GET("/notifications/log", forwardTo(notificationBase+"/notifications/log"))

	// Agent — ADMIN (JWT + Admin Role required)
	agentAdmin := newAdminGroup(protected, "/agent")
	agentAdmin.Any("", agent)
	agentAdmin.Any("/*any", agent)

	// =========================================================================
	// PROTECTED ROUTES — JWT required
	// =========================================================================

	// Auth — protected actions
	protected.POST("/auth/logout", authProxy)
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

	// Coupons — authenticated lookup of a single code
	protected.GET("/coupons/:code", coupons)

	// Shipping
	protected.POST("/shipping/rates", shipping)

	// =========================================================================
	// ADMIN ROUTES — JWT + admin role required
	// =========================================================================

	// Products — write (presign is under /bff/admin/products/presign — do not
	// register admin.GET("/products/presign") here; it conflicts with public /products/*any)
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
	// Auth — admin-only user creation (forward to auth-service)
	admin.POST("/auth/admin/users", authProxy)
}
