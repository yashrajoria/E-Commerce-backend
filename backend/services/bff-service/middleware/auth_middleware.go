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
			// Fallback to cookie-based user_id (set by API gateway)
			if v, err := c.Cookie("user_id"); err == nil && v != "" {
				userID = v
			}
		}

		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Missing User ID"})
			return
		}

		c.Set(UserContextKey, userID)
		c.Next()
	}
}

func GetUserID(c *gin.Context) (string, error) {
	val, exists := c.Get(UserContextKey)
	if !exists {
		return "", errors.New("user ID not found in context")
	}
	userID, ok := val.(string)
	if !ok || userID == "" {
		return "", errors.New("user ID has invalid type in context")
	}
	return userID, nil
}
