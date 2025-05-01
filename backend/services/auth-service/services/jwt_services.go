package services

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
)

var secretKey = []byte("your-secret-key")

// GenerateJWT generates a token with user ID, username, and role
func GenerateJWT(userID, username, role string) (string, error) {
	// Define claims (payload)
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"role":     role,                                  // Add role to JWT payload
		"exp":      time.Now().Add(time.Hour * 24).Unix(), // Token expires in 24 hours
	}

	// Create the token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign and return the token
	return token.SignedString(secretKey)
}

// GetUserIDFromJWT extracts and returns the user ID from the JWT token in the request cookie
func GetUserIDFromJWT(c *gin.Context) (string, error) {
	// Get the token from the cookie (you could also use Authorization header if preferred)
	tokenStr, err := c.Cookie("token")
	if err != nil {
		return "", errors.New("no token found")
	}

	// Parse and validate the JWT token
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		// Ensure the token method is HMAC-SHA256
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return secretKey, nil
	})

	// If the token is invalid or expired
	if err != nil || !token.Valid {
		return "", errors.New("invalid or expired token")
	}

	// Extract claims from the token
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("unable to parse claims")
	}

	// Extract user_id from claims
	userID, ok := claims["user_id"].(string)
	if !ok {
		return "", errors.New("user_id not found in token")
	}

	return userID, nil
}
