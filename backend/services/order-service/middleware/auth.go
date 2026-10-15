// Package middleware provides authentication and configuration injection middlewares for order routes.
package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const UserContextKey = "userID"

// AuthMiddleware returns a Gin middleware handler for request processing.
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

		c.Set("userID", userID)
		c.Set("role", strings.ToLower(strings.TrimSpace(role)))
		c.Set("email", email)

		c.Next()
	}
}

// GetUserID retrieves user id.
func GetUserID(c *gin.Context) (string, error) {
	if val, ok := c.Get(UserContextKey); ok {
		if id, ok := val.(string); ok && id != "" {
			return id, nil
		}
	}
	return "", errors.New("user ID not found in context")
}

// ConfigMiddleware injects configuration values into the Gin context, such as upstream service URLs.
func ConfigMiddleware(productServiceURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("product_service_url", productServiceURL)
		c.Next()
	}
}
// AdminOnly returns a Gin middleware handler for request processing.
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		roleVal, exists := c.Get("role")
		roleStr, _ := roleVal.(string)
		if !exists || roleStr != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin role required"})
			c.Abort()
			return
		}
		c.Next()
	}
}
