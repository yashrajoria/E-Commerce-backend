package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const UserKey = "userID"

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		c.Set(UserKey, userID)
		c.Next()
	}
}

func GetUserID(c *gin.Context) string {
	if val, exists := c.Get(UserKey); exists {
		return val.(string)
	}
	return ""
}
