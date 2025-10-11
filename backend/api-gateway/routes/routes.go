package routes

import (
	"api-gateway/middlewares"
	"api-gateway/utils"

	"github.com/gin-gonic/gin"
)

func RegisterAllRoutes(r *gin.Engine) {
	// ===== PUBLIC ROUTES =====
	public := r.Group("/")

	// Products routes - handle both /products and /products/*
	public.GET("/products", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://product-service:8082/products",
		})
	})
	public.GET("/products/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://product-service:8082/products",
		})
	})

	// Categories routes - handle both /categories and /categories/*
	public.GET("/categories", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://product-service:8082/categories",
		})
	})
	public.GET("/categories/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://product-service:8082/categories",
		})
	})

	// ===== AUTH ROUTES (PUBLIC) =====
	auth := r.Group("/auth")

	// Auth routes with wildcard
	auth.GET("/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://auth-service:8081",
		})
	})
	auth.POST("/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://auth-service:8081",
		})
	})

	// ===== PROTECTED ROUTES (JWT Required) =====
	protected := r.Group("/")
	protected.Use(middlewares.JWTMiddleware())

	// User routes - handle both /users and /users/*
	protected.GET("/users", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://user-service:8085/users",
		})
	})
	protected.GET("/users/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://user-service:8085/users",
		})
	})
	protected.POST("/users/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://user-service:8085/users",
		})
	})
	protected.PUT("/users/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://user-service:8085/users",
		})
	})
	protected.DELETE("/users/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://user-service:8085/users",
		})
	})

	// Cart routes - handle both /cart and /cart/*
	protected.GET("/cart", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://cart-service:8086/cart",
		})
	})
	protected.GET("/cart/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://cart-service:8086/cart",
		})
	})
	protected.POST("/cart/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://cart-service:8086/cart",
		})
	})
	protected.PUT("/cart/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://cart-service:8086/cart",
		})
	})
	protected.DELETE("/cart/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://cart-service:8086/cart",
		})
	})

	// Order routes - handle both /orders and /orders/*
	protected.GET("/orders", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://order-service:8083/orders",
		})
	})
	protected.GET("/orders/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://order-service:8083/orders",
		})
	})
	protected.POST("/orders", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://order-service:8083/orders",
		})
	})
	protected.POST("/orders/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://order-service:8083/orders",
		})
	})

	// ===== ADMIN ROUTES (JWT + Admin Role Required) =====
	admin := protected.Group("/")
	admin.Use(middlewares.AdminRoleMiddleware())

	// Admin product routes
	admin.POST("/products", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://product-service:8082/products",
		})
	})
	admin.POST("/products/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://product-service:8082/products",
		})
	})
	admin.PUT("/products/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://product-service:8082/products",
		})
	})
	admin.DELETE("/products/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://product-service:8082/products",
		})
	})

	// Admin category routes
	admin.POST("/categories", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://product-service:8082/categories",
		})
	})
	admin.POST("/categories/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://product-service:8082/categories",
		})
	})
	admin.PUT("/categories/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://product-service:8082/categories",
		})
	})
	admin.DELETE("/categories/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://product-service:8082/categories",
		})
	})

	// Admin order routes
	admin.PUT("/orders/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://order-service:8083/orders",
		})
	})
	admin.DELETE("/orders/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://order-service:8083/orders",
		})
	})

	// Payment routes (protected)
	protected.POST("/payment", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://payment-service:8087/payment",
		})
	})
	protected.POST("/payment/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://payment-service:8087/payment",
		})
	})
	protected.GET("/payment/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://payment-service:8087/payment",
		})
	})
}
