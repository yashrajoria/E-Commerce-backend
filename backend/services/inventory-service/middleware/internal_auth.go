package middleware

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

const InternalServiceTokenHeader = "X-Internal-Service-Token"

// RequireInternalServiceToken gates mesh-only inventory mutations.
// Kept local (inventory module does not depend on common yet).
func RequireInternalServiceToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		expected := strings.TrimSpace(os.Getenv("INTERNAL_SERVICE_TOKEN"))
		if expected == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "internal service auth is not configured",
			})
			return
		}

		provided := strings.TrimSpace(c.GetHeader(InternalServiceTokenHeader))
		if provided == "" ||
			subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid or missing internal service token",
			})
			return
		}
		c.Next()
	}
}
