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

// JWTMiddleware validates JWT access token and refreshes when needed
func JWTMiddleware() gin.HandlerFunc {
	_ = godotenv.Load()
	secretKey := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secretKey == "" {
		logger.Log.Fatal("JWT_SECRET is not set in env")
	}

	return func(c *gin.Context) {
		tokenString, err := c.Cookie("token")

		if err != nil || tokenString == "" {
			refreshToken, err := c.Cookie("refresh_token")
			if err != nil || refreshToken == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing authentication token"})
				c.Abort()
				return
			}

			token, err := jwt.Parse(refreshToken, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return []byte(secretKey), nil
			})

			if err != nil || token == nil || !token.Valid {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
				c.Abort()
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
				c.Abort()
				return
			}

			userID, _ := claims["user_id"].(string)
			email, _ := claims["email"].(string)
			role, _ := claims["role"].(string)

			accessToken, err := generateToken(userID, email, role, 15*time.Minute)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate new access token"})
				c.Abort()
				return
			}

			c.SetCookie("token", accessToken, 900, "/", "", true, true) // Secure, HttpOnly

			tokenString = accessToken
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(secretKey), nil
		})

		if err != nil || token == nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			userID, _ := claims["user_id"].(string)
			email, _ := claims["email"].(string)
			role, _ := claims["role"].(string)

			c.Set("user_id", userID)
			c.Set("email", email)
			c.Set("role", role)

			c.Request.Header.Set("X-User-ID", userID)
			c.Request.Header.Set("X-User-Email", email)
			c.Request.Header.Set("X-User-Role", role)
		}

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
	secretKey := []byte(os.Getenv("JWT_SECRET"))
	if len(secretKey) == 0 {
		return "", fmt.Errorf("JWT_SECRET not set")
	}

	claims := jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"role":    role,
		"exp":     time.Now().Add(expiration).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secretKey)
}
