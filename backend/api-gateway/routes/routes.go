package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/api-gateway/middlewares"
	"github.com/yashrajoria/api-gateway/utils"
)

func RegisterAllRoutes(r *gin.Engine) {
	// Public routes (GETs)
	public := r.Group("/")
	public.GET("/products/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{TargetBase: "http://product-service:8082/products"})
	})
	public.GET("/categories/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{TargetBase: "http://product-service:8082/categories"})
	})

	// Protected routes (all methods) with JWT
	protected := r.Group("/")
	protected.Use(middlewares.JWTMiddleware())

	// User routes (all protected)
	protected.GET("/users/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{TargetBase: "http://user-service:8085/users"})
	})
	protected.POST("/users/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{TargetBase: "http://user-service:8085/users"})
	})
	// similarly PUT, DELETE if needed...

	// Cart routes (protected)
	protected.GET("/cart/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{TargetBase: "http://cart-service:8086/cart"})
	})
	protected.POST("/cart/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{TargetBase: "http://cart-service:8086/cart"})
	})
	protected.DELETE("/cart/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{TargetBase: "http://cart-service:8086/cart"})
	})

	// Admin-only routes with role middleware
	admin := protected.Group("/")
	admin.Use(middlewares.AdminRoleMiddleware())
	admin.POST("/products/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{TargetBase: "http://product-service:8082/products"})
	})
	admin.PUT("/products/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{TargetBase: "http://product-service:8082/products"})
	})
	admin.DELETE("/products/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{TargetBase: "http://product-service:8082/products"})
	})

	admin.POST("/categories/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{TargetBase: "http://product-service:8082/categories"})
	})
	admin.PUT("/categories/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{TargetBase: "http://product-service:8082/categories"})
	})
	admin.DELETE("/categories/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{TargetBase: "http://product-service:8082/categories"})
	})

	// Auth routes (open or as per your auth service)
	auth := r.Group("/auth")
	auth.GET("/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{TargetBase: "http://auth-service:8081"})
	})
	auth.POST("/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{TargetBase: "http://auth-service:8081"})
	})
	// etc.
}
