package main

import (
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/api-gateway/logger"
	"github.com/yashrajoria/api-gateway/middlewares"
	"github.com/yashrajoria/api-gateway/routes"
)

// ✅ Fixed: forwardRequest properly ignores Access-Control headers and works with CORS middleware
func forwardRequest(c *gin.Context, targetBase string) {
	log.Println("Forwarding request to:", targetBase+c.Param("any"))
	targetURL := targetBase + c.Param("any")

	// Attach query strings if present
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	// Create new request
	req, err := http.NewRequest(c.Request.Method, targetURL, c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	// Copy requester’s headers into the forwarded request
	for k, v := range c.Request.Header {
		req.Header[k] = v
	}

	// Send request to downstream service
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error forwarding request:", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to reach target service"})
		return
	}
	defer resp.Body.Close()

	// ✅ Copy only non-CORS response headers (let gin-contrib/cors add them correctly)
	for k, v := range resp.Header {
		lower := strings.ToLower(k)
		if strings.HasPrefix(lower, "access-control-") {
			continue // don't copy CORS headers from downstream services
		}
		c.Header(k, strings.Join(v, ","))
	}

	// Set status and stream body to client
	c.Status(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}

func main() {
	r := gin.Default()
	logger.InitLogger()
	defer logger.Log.Sync()

	log.Println("Applying CORS middleware...")

	//Global CORS config allowing credentials and proper origin
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Public route
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "API Gateway Running"})
	})

	// Auth routes (login/signup etc.)
	routes.RegisterAuthRoutes(r)

	// ✅ Protected routes with JWT middleware
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
	protected.PUT("/products/*any", func(c *gin.Context) {
		forwardRequest(c, "http://product-service:8082/products")
	})
	protected.DELETE("/products/*any", func(c *gin.Context) {
		forwardRequest(c, "http://product-service:8082/products")
	})

	// 🛠️ FIXED /categories routes
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

	// INVENTORY SERVICE
	protected.GET("/inventory/*any", func(c *gin.Context) {
		forwardRequest(c, "http://localhost:8084")
	})

	// ORDER SERVICE
	protected.GET("/orders/*any", func(c *gin.Context) {
		forwardRequest(c, "http://order-service:8083/orders")
	})
	protected.POST("/orders/*any", func(c *gin.Context) {
		forwardRequest(c, "http://order-service:8083/orders")
	})

	// CART SERVICE
	protected.GET("/cart/*any", func(c *gin.Context) {
		forwardRequest(c, "http://cart-service:8086/cart")
	})
	protected.POST("/cart/*any", func(c *gin.Context) {
		forwardRequest(c, "http://cart-service:8086/cart")
	})
	protected.DELETE("/cart/*any", func(c *gin.Context) {
		forwardRequest(c, "http://cart-service:8086/cart")
	})

	// ✅ OPTIONAL: catch-all OPTIONS handler for safety (not required if cors middleware works)
	r.OPTIONS("/*path", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Start gateway
	r.Run(":8080")
}
