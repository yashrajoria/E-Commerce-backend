package services

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

// TokenPair holds the generated access and refresh tokens.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// TokenService is responsible for creating and validating JWTs.
type TokenService struct {
	secretKey []byte
}

// NewTokenService creates a new TokenService, loading the secret from the environment.
func NewTokenService() *TokenService {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		// The service cannot function without a secret, so it's appropriate to panic on startup.
		panic("JWT_SECRET environment variable not set")
	}
	return &TokenService{secretKey: []byte(secret)}
}

// GenerateTokenPair creates a new access and refresh token pair.
func (s *TokenService) GenerateTokenPair(userID, email, role string) (*TokenPair, string, error) {
	accessToken, err := s.generateToken(userID, email, role, "access", 15*time.Minute, "")
	if err != nil {
		return nil, "", err
	}

	// generate a unique token id for the refresh token (jti)
	tokenID := uuid.NewString()
	refreshToken, err := s.generateToken(userID, email, role, "refresh", 7*24*time.Hour, tokenID) // 7 days
	if err != nil {
		return nil, "", err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, tokenID, nil
}

// ValidateToken parses and validates any given token string.
func (s *TokenService) ValidateToken(tokenStr, expectedType string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.secretKey, nil
	})

	if err != nil || !token.Valid {
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

// generateToken is an internal helper to create a specific token.
func (s *TokenService) generateToken(userID, email, role, tokenType string, duration time.Duration, tokenID string) (string, error) {
	claims := jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"role":  role,
		"typ":   tokenType,
		"exp":   time.Now().Add(duration).Unix(),
		"iat":   time.Now().Unix(),
	}
	if tokenType == "refresh" && tokenID != "" {
		claims["jti"] = tokenID
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secretKey)
}
