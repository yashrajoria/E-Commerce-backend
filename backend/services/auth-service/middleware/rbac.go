package middlewares

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AdminOnly requires X-User-Role=admin (injected by the API gateway from the JWT).
// Do not trust client cookies for authorization on this service.
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := strings.TrimSpace(strings.ToLower(c.GetHeader("X-User-Role")))
		if role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Admin role required"})
			return
		}
		c.Next()
	}
}
