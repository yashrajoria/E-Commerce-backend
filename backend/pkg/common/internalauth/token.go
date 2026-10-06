package internalauth

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// Header is the shared mesh credential for service→service calls.
const Header = "X-Internal-Service-Token"

// Token returns the configured mesh token (empty if unset).
func Token() string {
	return strings.TrimSpace(os.Getenv("INTERNAL_SERVICE_TOKEN"))
}

// Apply sets the mesh token header when configured.
func Apply(req *http.Request) {
	if req == nil {
		return
	}
	token := Token()
	if token == "" {
		return
	}
	req.Header.Set(Header, token)
}

// Require rejects requests that lack a valid mesh token.
// Fail-closed: missing env configuration also rejects (misconfigured deploy).
func Require() gin.HandlerFunc {
	return func(c *gin.Context) {
		expected := Token()
		if expected == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "internal service auth is not configured",
			})
			return
		}

		provided := strings.TrimSpace(c.GetHeader(Header))
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
