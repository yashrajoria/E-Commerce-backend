package auth

import (
	"fmt"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"github.com/joho/godotenv"
)

var secretKey []byte

func init() {
	_ = godotenv.Load()
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secret == "" {
		// leave secretKey nil; callers will see parse error
		secretKey = nil
		return
	}
	secretKey = []byte(secret)
}

// ParseAndValidateToken parses a JWT token string and returns its claims.
// If expectedType is non-empty, the claim "typ" must match it.
func ParseAndValidateToken(tokenStr, expectedType string) (jwt.MapClaims, error) {
	if secretKey == nil {
		return nil, fmt.Errorf("JWT secret not configured")
	}

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
