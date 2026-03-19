package middlewares

import (
	"api-gateway/logger"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

var (
	secretKey    []byte
	isProduction bool
	cookieDomain string
)

func init() {
	_ = godotenv.Load()
	isProduction = os.Getenv("ENV") == "production"
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secret == "" {
		if isProduction {
			log.Fatal("JWT_SECRET is not set in env")
		}
		secret = "dev-insecure-jwt-secret"
		log.Printf("[GATEWAY][JWT] JWT_SECRET missing, using local dev fallback secret")
	}
	secretKey = []byte(secret)
	cookieDomain = os.Getenv("COOKIE_DOMAIN")
}

// JWTMiddleware validates JWT access token and refreshes when needed
func JWTMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Accept either __session (set by auth service) or token cookie
		tokenString, err := c.Cookie("__session")
		if err != nil || tokenString == "" {
			tokenString, _ = c.Cookie("token")
		}
		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing authentication token"})
			c.Abort()
			return
		}

		// validate final access token
		claims, err := parseToken(tokenString, "access")
		if err != nil {
			logger.Log.Warn("token parse error", zap.Error(err))
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		userID, _ := claims["sub"].(string)
		email, _ := claims["email"].(string)
		role, _ := claims["role"].(string)

		// log claims for debugging
		logger.Log.Debug("jwt claims parsed",
			zap.String("user_id", userID),
			zap.String("role", role),
		)

		c.Set("user_id", userID)
		c.Set("email", email)
		c.Set("role", role)

		c.Next()
	}
}

// AdminRoleMiddleware restricts access to users with role admin
func AdminRoleMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists || role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin role required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// parseToken validates and extracts claims
func parseToken(tokenStr, expectedType string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secretKey, nil
	})

	if err != nil || token == nil || !token.Valid {
		return nil, fmt.Errorf("invalid or expired token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}
	if expectedType != "" {
		if typ, ok := claims["typ"].(string); !ok || typ != expectedType {
			return nil, fmt.Errorf("invalid token type")
		}
	}

	return claims, nil
}
