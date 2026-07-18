package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	AdminUserIDContextKey   = "user_id"
	AdminUserRoleContextKey = "user_role"
)

// AdminAuthMiddleware allows requests only when user role is admin.
// Trusts gateway-injected X-User-Role only (no cookie fallback).
func AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
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
