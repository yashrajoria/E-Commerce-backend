package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/api-gateway/middlewares"
)

func main() {
	r := gin.Default()
	// Public route (no authentication needed)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "API Gateway Running"})
	})

	// ✅ Protected routes (Require JWT authentication)
	protected := r.Group("/")
	protected.Use(middlewares.JWTMiddleware()) // Apply JWT only to protected routes

	// Forward requests to Auth Service (No authentication needed for login/signup)
	r.GET("/user/*any", func(c *gin.Context) {
		// c.Redirect(http.StatusTemporaryRedirect, "http://auth-service:8081"+c.Param("any"))
		c.Redirect(http.StatusTemporaryRedirect, "http://localhost:8081"+c.Param("any"))
	})
	r.POST("/user/*any", func(c *gin.Context) {
		// c.Redirect(http.StatusTemporaryRedirect, "http://auth-service:8081"+c.Param("any"))
		c.Redirect(http.StatusTemporaryRedirect, "http://localhost:8081"+c.Param("any"))
	})

	// Forward requests to Product Service (Protected)
	protected.GET("/products/*any", func(c *gin.Context) {
		// c.Redirect(http.StatusTemporaryRedirect, "http://product-service:8082"+c.Param("any"))
		c.Redirect(http.StatusTemporaryRedirect, "http://localhost:8082"+c.Param("any"))
	})

	// Forward requests to Order Service (Protected)
	protected.GET("/orders/*any", func(c *gin.Context) {
		// c.Redirect(http.StatusTemporaryRedirect, "http://order-service:8083"+c.Param("any"))
		c.Redirect(http.StatusTemporaryRedirect, "http://localhost:8083"+c.Param("any"))
	})
	protected.GET("/inventory/*any", func(c *gin.Context) {
		// c.Redirect(http.StatusTemporaryRedirect, "http://order-service:8083"+c.Param("any"))
		c.Redirect(http.StatusTemporaryRedirect, "http://localhost:8084"+c.Param("any"))
	})

	r.Run(":8080") // API Gateway runs on port 8080
}
