package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

const UserContextKey = "userID"

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		role := c.GetHeader("X-User-Role")
		email := c.GetHeader("X-User-Email")

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}

		// Put them into Gin context
		c.Set("userID", userID)
		c.Set("role", role)
		c.Set("email", email)

		c.Next()
	}
}

func GetUserID(c *gin.Context) (string, error) {
	if val, ok := c.Get(UserContextKey); ok {
		if id, ok := val.(string); ok && id != "" {
			return id, nil
		}
	}
	return "", errors.New("user ID not found in context")
}

// Injects any config value into Gin context; here for product service URL
func ConfigMiddleware(productServiceURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("product_service_url", productServiceURL)
		c.Next()
	}
}
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists || role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin role required"})
			c.Abort()
			return
		}
		c.Next()
	}
}
