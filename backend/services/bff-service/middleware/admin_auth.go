package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/common/internalauth"
)

const (
	AdminUserIDContextKey   = "user_id"
	AdminUserRoleContextKey = "user_role"
)

// AdminAuthMiddleware allows requests only when they carry both a valid
// internal mesh token (proving the call came through api-gateway, which is
// the only party that sets INTERNAL_SERVICE_TOKEN on outbound requests) and
// an X-User-Role of admin. The mesh-token check is defense in depth: without
// it, X-User-Role alone would grant admin access to anyone able to reach this
// service directly (e.g. a compose network misconfiguration).
func AdminAuthMiddleware() gin.HandlerFunc {
	requireInternal := internalauth.Require()
	return func(c *gin.Context) {
		requireInternal(c)
		if c.IsAborted() {
			return
		}

		userRole := strings.ToLower(strings.TrimSpace(c.GetHeader("X-User-Role")))
		if userRole != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "forbidden",
			})
			return
		}

		userID := c.GetHeader("X-User-ID")
		c.Set(AdminUserIDContextKey, userID)
		c.Set(AdminUserRoleContextKey, userRole)
		c.Next()
	}
}
