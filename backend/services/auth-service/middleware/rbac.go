package middlewares

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/common/internalauth"
)

// AdminOnly requires X-User-Role=admin (injected by the API gateway from the
// JWT) AND a valid internal mesh token, proving the request actually came
// through api-gateway rather than being sent directly with a forged header.
// Do not trust client cookies for authorization on this service.
func AdminOnly() gin.HandlerFunc {
	requireInternal := internalauth.Require()
	return func(c *gin.Context) {
		requireInternal(c)
		if c.IsAborted() {
			return
		}

		role := strings.TrimSpace(strings.ToLower(c.GetHeader("X-User-Role")))
		if role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Admin role required"})
			return
		}
		c.Next()
	}
}
