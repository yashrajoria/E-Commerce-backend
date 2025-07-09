package middlewares

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/joho/godotenv"
)

// JWTMiddleware checks and validates JWT token from cookies
func JWTMiddleware() gin.HandlerFunc {
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ No .env file found, falling back to system environment variables")
	}
	secretKey := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secretKey == "" {
		log.Fatalln("JWT_SECRET not set in environment variables")
	}
	return func(c *gin.Context) {
		// Try to get access token first
		tokenString, err := c.Cookie("token")
		if err != nil || tokenString == "" {
			// If access token is missing, try refresh token
			refreshToken, err := c.Cookie("refresh_token")
			if err != nil || refreshToken == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing authentication token"})
				c.Abort()
				return
			}

			// Validate refresh token
			token, err := jwt.Parse(refreshToken, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return []byte(secretKey), nil
			})

			if err != nil || !token.Valid {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
				c.Abort()
				return
			}

			// Extract claims from refresh token
			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
				c.Abort()
				return
			}

			// Generate new token pair
			userID := claims["user_id"].(string)
			email := claims["email"].(string)
			role := claims["role"].(string)

			// Generate new access token (15 minutes)
			accessToken, err := generateToken(userID, email, role, 15*time.Minute)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate new access token"})
				c.Abort()
				return
			}

			// Set new access token in cookie
			c.SetCookie("token", accessToken, 900, "/", "", false, true) // 15 minutes
			tokenString = accessToken
		}

		// Validate access token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(secretKey), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			c.Set("user_id", claims["user_id"])
			c.Set("email", claims["email"])
			c.Set("role", claims["role"])

			c.Request.Header.Set("X-User-ID", claims["user_id"].(string))

		}

		c.Next()
	}
}

// generateToken creates a JWT token with the given claims and expiration
func generateToken(userID string, email string, role string, expiration time.Duration) (string, error) {
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
