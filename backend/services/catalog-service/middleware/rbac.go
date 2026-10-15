// Package middleware provides role-based access control (RBAC) middlewares for catalog operations.
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AdminOnly requires gateway-injected X-User-Role=admin.
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := strings.ToLower(strings.TrimSpace(c.GetHeader("X-User-Role")))
		if role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Admin role required"})
			return
		}
		c.Next()
	}
}
