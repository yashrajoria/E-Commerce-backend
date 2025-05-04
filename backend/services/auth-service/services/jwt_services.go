package services

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/joho/godotenv"
)

var secretKey = []byte(os.Getenv("JWT_SECRET"))

func init() {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found, using system environment variables")
	}
	if len(secretKey) == 0 {
		log.Fatal("Missing JWT_SECRET in environment variables")
	}
}

// GenerateJWT generates a token with user ID, username, and role
func GenerateJWT(userID, username, role string) (string, error) {
	// Define claims (payload)
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"role":     role,
		"exp":      time.Now().Add(time.Hour * 24).Unix(), // Can add buffer as needed
	}

	// Create the token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign and return the token
	return token.SignedString(secretKey)
}

// GetUserIDFromJWT extracts and returns the user ID from the JWT token in the request cookie
func GetUserIDFromJWT(c *gin.Context) (string, error) {
	// Get the token from the cookie
	tokenStr, err := c.Cookie("token")
	if err != nil {
		return "", fmt.Errorf("no token found: %v", err)
	}

	// Parse and validate the JWT token
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		// Ensure the token method is HMAC-SHA256
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secretKey, nil
	})

	// If the token is invalid or expired
	if err != nil || !token.Valid {
		return "", fmt.Errorf("invalid or expired token: %v", err)
	}

	// Extract claims from the token
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("unable to parse claims from token")
	}

	// Extract user_id from claims
	userID, ok := claims["user_id"].(string)
	if !ok {
		return "", fmt.Errorf("user_id not found in token claims")
	}

	return userID, nil
}
