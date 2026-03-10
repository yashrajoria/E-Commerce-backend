package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

const UserContextKey = "userID"

// AuthMiddleware reads identity headers injected by the API gateway.
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		role := c.GetHeader("X-User-Role")
		email := c.GetHeader("X-User-Email")

		// Fallback to cookies (set by API gateway) if headers missing
		if userID == "" {
			if v, err := c.Cookie("user_id"); err == nil && v != "" {
				userID = v
			}
		}
		if role == "" {
			if v, err := c.Cookie("user_role"); err == nil && v != "" {
				role = v
			}
		}
		if email == "" {
			if v, err := c.Cookie("user_email"); err == nil && v != "" {
				email = v
			}
		}

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}

		c.Set("userID", userID)
		c.Set("role", role)
		c.Set("email", email)
		c.Next()
	}
}

// GetUserID extracts the user ID from the Gin context.
func GetUserID(c *gin.Context) (string, error) {
	if val, ok := c.Get(UserContextKey); ok {
		if id, ok := val.(string); ok && id != "" {
			return id, nil
		}
	}
	return "", errors.New("user ID not found in context")
}

// AdminOnly restricts access to admin role.
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
