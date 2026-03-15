package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	AdminUserIDContextKey   = "user_id"
	AdminUserRoleContextKey = "user_role"
)

// AdminAuthMiddleware allows requests only when X-User-Role is admin.
func AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole := c.GetHeader("X-User-Role")
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
