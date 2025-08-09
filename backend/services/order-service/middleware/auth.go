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
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		c.Set(UserContextKey, userID)
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
