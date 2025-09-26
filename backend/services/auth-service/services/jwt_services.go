package services

import (
	"fmt"
	"os"
	"time"

	"log"

	"github.com/golang-jwt/jwt/v4"
	"github.com/joho/godotenv"
)

var secretKey []byte

func init() {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found, using system environment variables")
	}
	secretKey = []byte(os.Getenv("JWT_SECRET"))
	if len(secretKey) == 0 {
		log.Println("Missing JWT_SECRET in environment variables")
	}
}

// TokenPair holds access and refresh tokens
type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// GenerateTokenPair creates a short-lived access token and a long-lived refresh token
func GenerateTokenPair(userID, email, role string) (*TokenPair, error) {
	accessToken, err := generateToken(userID, email, role, 15*time.Minute) // Access token: 15 mins
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := generateToken(userID, email, role, 7*24*time.Hour) // Refresh token: 7 days
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// generateToken is a helper to create a JWT token with given expiry duration
func generateToken(userID, email, role string, duration time.Duration) (string, error) {
	claims := jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"role":  role,
		"exp":   time.Now().Add(duration).Unix(),
		"iat":   time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secretKey)
}

// ValidateRefreshToken parses and validates a refresh token string, returning claims if valid
func ValidateRefreshToken(tokenStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secretKey, nil
	})

	if err != nil || token == nil || !token.Valid {
		return nil, fmt.Errorf("invalid or expired refresh token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// RefreshTokens generates a new pair using a valid refresh token
func RefreshTokens(refreshToken string) (*TokenPair, error) {
	claims, err := ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, err
	}

	userID, _ := claims["sub"].(string)
	email, _ := claims["email"].(string)
	role, _ := claims["role"].(string)

	return GenerateTokenPair(userID, email, role)
}
