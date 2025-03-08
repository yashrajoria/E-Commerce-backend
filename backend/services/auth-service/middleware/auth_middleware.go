package middlewares

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
)

// JWTSecret should match the one in `services/jwt.go`
var JWTSecret = []byte("your-secret-key")

// RequireRole middleware checks if the user has the required role
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get the token from headers
		tokenString := c.GetHeader("Authorization")
		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing token"})
			c.Abort()
			return
		}

		// Parse the token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return JWTSecret, nil
		})
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		// Extract claims and check role
		if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
			userRole := claims["role"].(string)
			if userRole != role {
				c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
				c.Abort()
				return
			}

			// Proceed if role matches
			c.Next()
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}
	}
}
