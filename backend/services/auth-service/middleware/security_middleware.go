package middlewares

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/time/rate"
)

// TokenPair represents both access and refresh tokens
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// RateLimiter stores rate limiters for different IPs
type RateLimiter struct {
	ips    map[string]*rate.Limiter
	mu     *sync.RWMutex
	rate   rate.Limit
	burst  int
	ttl    time.Duration
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(r rate.Limit, b int, ttl time.Duration) *RateLimiter {
	return &RateLimiter{
		ips:    make(map[string]*rate.Limiter),
		mu:     &sync.RWMutex{},
		rate:   r,
		burst:  b,
		ttl:    ttl,
	}
}

// GetLimiter returns the rate limiter for the given IP
func (rl *RateLimiter) GetLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.ips[ip]
	if !exists {
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.ips[ip] = limiter
	}

	return limiter
}

// RateLimitMiddleware creates a rate limiting middleware
func RateLimitMiddleware() gin.HandlerFunc {
	// Create a rate limiter: 100 requests per minute with burst of 50
	limiter := NewRateLimiter(rate.Limit(100), 50, time.Minute*5)

	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !limiter.GetLimiter(ip).Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded. Please try again later.",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// GenerateTokenPair generates both access and refresh tokens
func GenerateTokenPair(userID string, email string, role string) (*TokenPair, error) {
	// Generate access token (short-lived, 15 minutes)
	accessToken, err := generateToken(userID, email, role, 15*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	// Generate refresh token (long-lived, 7 days)
	refreshToken, err := generateToken(userID, email, role, 24*7*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
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

// RefreshTokenMiddleware handles token refresh
func RefreshTokenMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		refreshToken, err := c.Cookie("refresh_token")
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token not found"})
			c.Abort()
			return
		}

		// Parse and validate the refresh token
		token, err := jwt.Parse(refreshToken, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(os.Getenv("JWT_SECRET")), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
			c.Abort()
			return
		}

		// Extract claims
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

		newTokens, err := GenerateTokenPair(userID, email, role)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate new tokens"})
			c.Abort()
			return
		}

		// Set new tokens in cookies
		c.SetCookie("token", newTokens.AccessToken, 900, "/", "", false, true)  // 15 minutes
		c.SetCookie("refresh_token", newTokens.RefreshToken, 604800, "/", "", false, true)  // 7 days

		c.Next()
	}
} 