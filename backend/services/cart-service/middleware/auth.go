package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// RequireUser requires gateway-injected X-User-ID (JWT identity).
// Closes mesh bypass when cart routes are hit without going through the gateway.
func RequireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := strings.TrimSpace(c.GetHeader("X-User-ID"))
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user not authorized"})
			return
		}
		c.Set("user_id", userID)
		c.Next()
	}
}
