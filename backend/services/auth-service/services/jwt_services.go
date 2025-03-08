package services

import (
	"time"

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
