package routes

import (
	"api-gateway/middlewares"
	"api-gateway/utils"

	"github.com/gin-gonic/gin"
)

func RegisterAllRoutes(r *gin.Engine) {
	forwardTo := func(targetBase string) gin.HandlerFunc {
		return func(c *gin.Context) {
			utils.ForwardRequest(c, utils.ForwardOptions{
				TargetBase: targetBase,
			})
		}
	}

	// BFF forwarding
	bff := forwardTo("http://bff-service:8088/bff")

	// ===== PUBLIC ROUTES =====
	public := r.Group("/")

	// Products routes - handle both /products and /products/*
	products := forwardTo("http://product-service:8082/products")
	public.GET("/products", products)
	public.GET("/products/*any", products)

	// Categories routes - handle both /categories and /categories/*
	categories := forwardTo("http://product-service:8082/categories")
	public.GET("/categories", categories)
	public.GET("/categories/*any", categories)

	// BFF public routes (pass-through to bff-service)
	public.GET("/bff", bff)
	// public.GET("/bff/*any", bff)

	// Expose docs at gateway root by forwarding to the BFF docs path
	public.GET("/docs", forwardTo("http://bff-service:8088/docs"))
	public.GET("/docs/*any", forwardTo("http://bff-service:8088/docs"))

	// ===== AUTH ROUTES (PUBLIC) =====
	// ===== PROTECTED ROUTES (JWT Required) =====
	protected := r.Group("/")
	protected.Use(middlewares.JWTMiddleware())
	auth := r.Group("/auth")
	authProxy := forwardTo("http://auth-service:8081/auth")

	// Auth routes with wildcard
	protected.GET("/auth/*any", authProxy)
	auth.POST("/*any", authProxy)

	// User routes - handle both /users and /users/*
	users := forwardTo("http://user-service:8085/users")
	protected.GET("/users", users)
	protected.GET("/users/*any", users)
	protected.POST("/users/*any", users)
	protected.PUT("/users/*any", users)
	protected.DELETE("/users/*any", users)

	// Cart routes - handle both /cart and /cart/*
	cart := forwardTo("http://cart-service:8086/cart")
	protected.GET("/cart", cart)
	protected.GET("/cart/*any", cart)
	protected.POST("/cart/*any", cart)
	protected.PUT("/cart/*any", cart)
	protected.DELETE("/cart/*any", cart)

	// Order routes - handle both /orders and /orders/*
	orders := forwardTo("http://order-service:8083/orders")
	protected.GET("/orders", orders)
	protected.GET("/orders/*any", orders)
	protected.POST("/orders", orders)
	protected.POST("/orders/*any", orders)

	// ===== ADMIN ROUTES (JWT + Admin Role Required) =====
	admin := protected.Group("/")
	admin.Use(middlewares.AdminRoleMiddleware())

	// Admin product routes
	admin.POST("/products", products)
	admin.POST("/products/*any", products)
	admin.PUT("/products/*any", products)
	admin.DELETE("/products/*any", products)

	// Admin category routes
	admin.POST("/categories", categories)
	admin.POST("/categories/*any", categories)
	admin.PUT("/categories/*any", categories)
	admin.DELETE("/categories/*any", categories)

	// Admin order routes
	admin.PUT("/orders/*any", orders)
	admin.DELETE("/orders/*any", orders)

	// Payment routes (protected)
	payment := forwardTo("http://payment-service:8087/payment")
	protected.POST("/payment", payment)
	protected.POST("/payment/*any", payment)
	protected.GET("/payment/*any", payment)

	// BFF: forward POSTs (protected) so POST actions (cart add/checkout) reach bff-service
	protected.POST("/bff/*any", bff)
	protected.POST("/bff", bff)

	// Fix: Explicitly protect BFF profile so Gateway parses the cookie and sets X-User-ID
	protected.GET("/bff/*any", bff)

	// Note: public GETs for `/bff` remain handled above so public pages still work.

	// Inventory routes
	inventory := forwardTo("http://inventory-service:8084/inventory")
	// Protected: read & operations
	protected.GET("/inventory/:productId", inventory)
	protected.POST("/inventory/check", inventory)
	// Admin: create & update stock
	admin.POST("/inventory", inventory)
	admin.PUT("/inventory/:productId", inventory)

	// Stripe webhook (public)
	public.POST("/stripe/webhook", forwardTo("http://payment-service:8087/stripe/webhook"))
}
