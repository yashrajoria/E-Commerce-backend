package middlewares

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/joho/godotenv"
	"github.com/yashrajoria/api-gateway/logger"
)

var secretKey []byte

func init() {
	_ = godotenv.Load()
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secret == "" {
		logger.Log.Fatal("JWT_SECRET is not set in env")
	}
	secretKey = []byte(secret)
}

// JWTMiddleware validates JWT access token and refreshes when needed
func JWTMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, err := c.Cookie("token")

		if err != nil || tokenString == "" {
			// Try refresh token
			refreshToken, err := c.Cookie("refresh_token")
			if err != nil || refreshToken == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing authentication token"})
				c.Abort()
				return
			}

			claims, err := parseToken(refreshToken)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
				c.Abort()
				return
			}

			userID, _ := claims["sub"].(string)
			email, _ := claims["email"].(string)
			role, _ := claims["role"].(string)

			// generate new access token
			accessToken, err := generateToken(userID, email, role, 15*time.Minute)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate new access token"})
				c.Abort()
				return
			}

			// set cookies
			c.SetCookie("token", accessToken, 900, "/", "", true, true)                // 15 min
			c.SetCookie("refresh_token", refreshToken, 7*24*3600, "/", "", true, true) // keep same refresh token

			tokenString = accessToken
		}

		// validate final access token
		claims, err := parseToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		userID, _ := claims["sub"].(string)
		email, _ := claims["email"].(string)
		role, _ := claims["role"].(string)

		c.Set("user_id", userID)
		c.Set("email", email)
		c.Set("role", role)

		c.Request.Header.Set("X-User-ID", userID)
		c.Request.Header.Set("X-User-Email", email)
		c.Request.Header.Set("X-User-Role", role)

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

// generateToken creates signed JWT token with claims
func generateToken(userID, email, role string, expiration time.Duration) (string, error) {
	claims := jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"role":  role,
		"exp":   time.Now().Add(expiration).Unix(),
		"iat":   time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secretKey)
}

// parseToken validates and extracts claims
func parseToken(tokenStr string) (jwt.MapClaims, error) {
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

	return claims, nil
}
