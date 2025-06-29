package main

import (
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/api-gateway/middlewares"
)

func forwardRequest(c *gin.Context, targetBase string) {
	log.Println("Forwarding request to:", targetBase+c.Param("any"))
	targetURL := targetBase + c.Param("any")

	// Append query string if present
	queryString := c.Request.URL.RawQuery
	if queryString != "" {
		targetURL = targetURL + "?" + queryString
	}

	req, err := http.NewRequest(c.Request.Method, targetURL, c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	// Copy headers
	for k, v := range c.Request.Header {
		req.Header[k] = v
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error forwarding request:", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to reach target service"})
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, v := range resp.Header {
		c.Header(k, strings.Join(v, ","))
	}

	c.Status(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}
func main() {
	r := gin.Default()
	log.Println("Applying CORS middleware...")

	// CORS Configuration
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000"}, // Ensure this matches frontend
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Public route (no authentication needed)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "API Gateway Running"})
	})

	// Forward requests to Auth Service (No authentication needed for login/signup)
	authGroup := r.Group("/auth")
	authGroup.OPTIONS("/*any", func(c *gin.Context) {
		log.Println("Handling OPTIONS request for:", c.Request.URL.Path)
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "http://localhost:3000"
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Status(http.StatusOK)
	})
	authGroup.GET("/*any", func(c *gin.Context) {
		forwardRequest(c, "http://auth-service:8081"+c.Param("any"))
	})
	authGroup.POST("/*any", func(c *gin.Context) {
		forwardRequest(c, "http://auth-service:8081")

	})
	authGroup.PUT("/*any", func(c *gin.Context) {
		forwardRequest(c, "http://auth-service:8081")

	})

	// Protected routes (Require JWT authentication)
	protected := r.Group("/")
	protected.Use(middlewares.JWTMiddleware())

	protected.GET("/users/*any", func(ctx *gin.Context) {
		forwardRequest(ctx, "http://user-service:8085/users")
	})
	protected.PUT("/users/*any", func(ctx *gin.Context) {
		forwardRequest(ctx, "http://user-service:8085/users")
	})
	protected.POST("/users/*any", func(ctx *gin.Context) {
		forwardRequest(ctx, "http://user-service:8085/users")
	})

	// Forward requests to Product Service (Protected)
	protected.GET("/products/*any", func(c *gin.Context) {
		forwardRequest(c, "http://product-service:8082/products")
	})

	protected.POST("/products/*any", func(c *gin.Context) {
		forwardRequest(c, "http://product-service:8082/products")
	})

	protected.DELETE("/products/*any", func(c *gin.Context) {
		forwardRequest(c, "http://product-service:8082/products")
	})

	protected.PUT("/products/*any", func(c *gin.Context) {
		forwardRequest(c, "http://product-service:8082/products")
	})

	protected.GET("/categories/*any", func(c *gin.Context) {
		forwardRequest(c, "http://product-service:8082/categories")
	})

	protected.POST("/categories/*any", func(c *gin.Context) {
		forwardRequest(c, "http://product-service:8082/categories")
	})

	protected.PUT("/categories/*any", func(c *gin.Context) {
		forwardRequest(c, "http://product-service:8082/categories")
	})

	protected.DELETE("/categories/*any", func(c *gin.Context) {
		forwardRequest(c, "http://product-service:8082/categories")
	})

	protected.GET("/inventory/*any", func(c *gin.Context) {
		forwardRequest(c, "http://localhost:8084"+c.Param("any"))
	})

	protected.POST("/orders/*any", func(c *gin.Context) {
		forwardRequest(c, "http://order-service:8083/orders")
	})

	protected.GET("/orders/*any", func(c *gin.Context) {
		forwardRequest(c, "http://order-service:8083/orders")
	})

	r.Run(":8080") // API Gateway runs on port 8080
}
