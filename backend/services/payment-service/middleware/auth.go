package middleware

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	jwt "github.com/golang-jwt/jwt/v4"
)

const UserKey = "userID"

// AuthMiddleware accepts either an X-User-ID header (dev/test) or a
// __session cookie containing a signed JWT. The JWT is validated using
// the JWT_SECRET env var and the `sub` claim is used as the user id.
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		if userID == "" {
			// First try gateway-propagated user_id cookie
			if v, err := c.Cookie("user_id"); err == nil && v != "" {
				userID = v
			}
		}
		if userID == "" {
			// Try cookie-based auth via __session JWT
			tokenStr, err := c.Cookie("__session")
			if err != nil || tokenStr == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
				return
			}

			secret := os.Getenv("JWT_SECRET")
			if secret == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
				return
			}

			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
				// Only allow HMAC signing for our cookie tokens
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
				return
			}

			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				if sub, ok := claims["sub"].(string); ok && sub != "" {
					userID = sub
				} else if email, ok := claims["email"].(string); ok && email != "" {
					// fallback to email if sub is not present
					userID = email
				}
			}

			if userID == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
				return
			}
		}

		c.Set(UserKey, userID)
		c.Next()
	}
}

func GetUserID(c *gin.Context) string {
	if val, exists := c.Get(UserKey); exists {
		return val.(string)
	}
	return ""
}
