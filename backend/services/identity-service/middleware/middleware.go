package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/common/internalauth"
)

const UserContextKey = "userID"

// AuthMiddleware trusts only the X-User-ID header injected by api-gateway's
// JWTMiddleware (backed by a validated JWT), with a cookie fallback for
// direct same-origin callers. Never trust client-supplied role claims here.
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		if userID == "" {
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
