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
	// Catalog absorbed product-service (:8082), inventory-service (:8084) and
	// cart-service (:8086): the old *_SERVICE_URL vars are kept as overrides
	// but all default to catalog-service:8082.
	productBase := getEnv("PRODUCT_SERVICE_URL", "http://catalog-service:8082")
	userBase := getEnv("USER_SERVICE_URL", "http://identity-service:8081")
	cartBase := getEnv("CART_SERVICE_URL", "http://catalog-service:8082")
	orderBase := getEnv("ORDER_SERVICE_URL", "http://order-service:8083")
	inventoryBase := getEnv("INVENTORY_SERVICE_URL", "http://catalog-service:8082")
	authBase := getEnv("AUTH_SERVICE_URL", "http://identity-service:8081")
	notificationBase := getEnv("NOTIFICATION_SERVICE_URL", "http://notification-service:8092")
	agentBase := getEnv("AGENT_SERVICE_URL", "http://agent-service:8000")

	// ── service targets ───────────────────────────────────────────────────────
	products := forwardTo(productBase + "/products")
	categories := forwardTo(productBase + "/categories")
	users := forwardTo(userBase + "/users")
	cart := forwardTo(cartBase + "/cart")
	orders := forwardTo(orderBase + "/orders")
	payment := forwardTo(orderBase + "/payment")
	inventory := forwardTo(inventoryBase + "/inventory")
	coupons := forwardTo(orderBase + "/coupons")
	promotions := forwardTo(orderBase + "/promotions")
	shipping := forwardTo(orderBase + "/shipping")
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

	// Stripe webhook — Stripe calls this directly, no auth
	public.POST("/stripe/webhook", forwardTo(orderBase+"/stripe/webhook"))

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
	// BFF alias used by admin/storefront SSR (proxyAuthAction, ssrAuth,
	// requireAdminApi call /bff/auth/refresh on the gateway).
	// Exact target (not authProxy): authProxy derives suffix from the
	// path, which fails for the /bff prefix — this forwards to /auth/refresh.
	public.POST("/bff/auth/refresh", forwardTo(authBase+"/auth/refresh"))

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

	// Flash sale waiting room — public entrance & queue status
	public.POST("/inventory/flash-sale/enter", inventory)
	public.GET("/inventory/flash-sale/status", inventory)

	// Dynamic Basket Optimizer — public / guest cart incentives
	public.POST("/promotions/optimize-basket", promotions)

	// ── Storefront aggregation (ex-BFF) ─────────────────────────────────────
	// bff-service is deleted. Its storefront fan-outs (home, profile,
	// checkout orchestration) are gone: callers use the domain routes below
	// directly (GET /products + /categories, GET /cart, POST /cart/checkout
	// then poll GET /payment/status/by-order/:order_id).
	// The /bff/admin/* prefix is retained as the admin/analytics contract
	// (agent-service + admin UI): pure proxies forward straight to the owning
	// service, dashboard aggregation lives in order-service.

	// BFF — ADMIN (JWT + Admin Role required)
	bffAdmin := newAdminGroup(protected, "/bff/admin")

	// Dashboard aggregation (order-service fans out in-process + mesh)
	bffAdmin.GET("/dashboard", forwardTo(orderBase+"/orders/admin/dashboard"))
	// Analytics reports (pure proxies to the owning service)
	bffAdmin.GET("/reports/sales", forwardTo(orderBase+"/orders/admin/stats"))
	bffAdmin.GET("/reports/users", users)
	bffAdmin.GET("/reports/inventory", inventory)

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

	// Agent BFF aliases (frontend contract: admin-new pages/api/agent/*
	// tries bff/admin/agent/* then bff/agent/* then agent/*).
	// Without these the admin AI assistant gets 404 on tools/query/session.
	// Single wildcard per group (gin panics on static + wildcard siblings).
	bffAgent := protected.Group("/bff/agent")
	bffAgent.Any("/*any", agent)
	bffAdmin.Any("/agent/*any", agent)

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
	protected.POST("/inventory/flash-sale/claim", inventory)
	protected.POST("/inventory/flash-sale/release", inventory)

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
	admin.POST("/inventory/flash-sale/configure", inventory)
	admin.POST("/inventory/flash-sale/reset", inventory)

	// Coupons — write
	admin.POST("/coupons", coupons)
	admin.GET("/coupons", coupons)
	admin.DELETE("/coupons/:code", coupons)

	// Notifications — admin read
	admin.GET("/notifications/log", notifications)
	// Auth — admin-only user creation (forward to identity-service)
	admin.POST("/auth/admin/users", authProxy)
}
