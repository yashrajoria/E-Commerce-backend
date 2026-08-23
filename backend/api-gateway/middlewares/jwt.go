package middlewares

import (
	"api-gateway/logger"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

var (
	secretKey       []byte
	authBaseURL     string
	refreshHTTP     = &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
	refreshInflight singleflight.Group
)

func InitJWTConfig() error {
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secret == "" {
		return fmt.Errorf("JWT_SECRET is not set in env")
	}
	secretKey = []byte(secret)
	authBaseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("AUTH_SERVICE_URL")), "/")
	if authBaseURL == "" {
		authBaseURL = "http://auth-service:8081"
	}
	return nil
}

// JWTMiddleware validates JWT access token and refreshes when needed
func JWTMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var tokenString string

		// 1. Try Authorization header (Bearer token)
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			tokenString = strings.TrimPrefix(authHeader, "Bearer ")
		}

		// 2. Fallback to cookies if header is missing
		if tokenString == "" {
			if cookie, err := c.Cookie("__session"); err == nil && cookie != "" {
				tokenString = cookie
			} else if cookie, err := c.Cookie("token"); err == nil && cookie != "" {
				tokenString = cookie
			}
		}

		if tokenString == "" {
			// No access token — try refresh_token before failing.
			if claims, ok := trySilentRefresh(c); ok {
				if !applyClaims(c, claims) {
					return
				}
				c.Next()
				return
			}
			logger.Log.Warn("authentication failed: missing token",
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method),
				zap.String("ip", c.ClientIP()),
			)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing authentication token"})
			c.Abort()
			return
		}

		// validate final access token
		claims, err := parseToken(tokenString, "access")
		if err != nil {
			// Access expired/invalid — rotate via refresh_token when present.
			if refreshed, ok := trySilentRefresh(c); ok {
				if !applyClaims(c, refreshed) {
					return
				}
				c.Next()
				return
			}
			logger.Log.Warn("authentication failed: invalid or expired token",
				zap.Error(err),
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method),
				zap.String("ip", c.ClientIP()),
			)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		if !applyClaims(c, claims) {
			return
		}
		c.Next()
	}
}

func applyClaims(c *gin.Context, claims jwt.MapClaims) bool {
	userID, _ := claims["sub"].(string)
	email, _ := claims["email"].(string)
	role, _ := claims["role"].(string)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token subject"})
		c.Abort()
		return false
	}

	logger.Log.Debug("jwt claims parsed",
		zap.String("user_id", userID),
		zap.String("role", role),
	)

	c.Set("user_id", userID)
	c.Set("email", email)
	c.Set("role", strings.ToLower(strings.TrimSpace(role)))
	return true
}

// refreshResult carries everything trySilentRefresh's caller needs back out
// of the (possibly shared, via singleflight) auth-service call.
type refreshResult struct {
	claims     jwt.MapClaims
	setCookies []string
}

// trySilentRefresh calls auth-service refresh using the refresh_token cookie and
// forwards rotated Set-Cookie headers to the browser. Concurrent requests that
// share the same refresh token are coalesced via singleflight so an expired
// token shared across many in-flight requests doesn't trigger a thundering
// herd of refresh calls (and doesn't race an auth-service that rotates/
// single-uses refresh tokens).
func trySilentRefresh(c *gin.Context) (jwt.MapClaims, bool) {
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil || strings.TrimSpace(refreshToken) == "" {
		return nil, false
	}

	correlationID := c.GetString("CorrelationID")
	path := c.Request.URL.Path

	v, err, _ := refreshInflight.Do(refreshToken, func() (any, error) {
		return doRefresh(refreshToken, correlationID, path)
	})
	if err != nil {
		return nil, false
	}

	result := v.(refreshResult)
	for _, raw := range result.setCookies {
		if raw != "" {
			c.Writer.Header().Add("Set-Cookie", raw)
		}
	}
	return result.claims, true
}

// doRefresh performs the actual HTTP call to auth-service. It intentionally
// uses its own bounded context (not the initiating request's context) since
// singleflight may share this call's result with other in-flight requests
// whose own contexts must not cancel it.
func doRefresh(refreshToken, correlationID, path string) (refreshResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authBaseURL+"/auth/refresh", nil)
	if err != nil {
		return refreshResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if correlationID != "" {
		req.Header.Set("X-Correlation-ID", correlationID)
	}
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshToken, Path: "/"})

	resp, err := refreshHTTP.Do(req)
	if err != nil {
		logger.Log.Warn("silent refresh failed: auth-service unreachable", zap.Error(err))
		return refreshResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		logger.Log.Warn("silent refresh rejected",
			zap.Int("status", resp.StatusCode),
			zap.String("path", path),
		)
		return refreshResult{}, fmt.Errorf("silent refresh rejected: status %d", resp.StatusCode)
	}

	var accessToken string
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "__session" && cookie.Value != "" {
			accessToken = cookie.Value
		}
	}
	if accessToken == "" {
		return refreshResult{}, fmt.Errorf("silent refresh response missing __session cookie")
	}

	claims, err := parseToken(accessToken, "access")
	if err != nil {
		return refreshResult{}, err
	}

	return refreshResult{claims: claims, setCookies: resp.Header.Values("Set-Cookie")}, nil
}

// AdminRoleMiddleware restricts access to users with role admin
func AdminRoleMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		roleVal, exists := c.Get("role")
		roleStr, ok := roleVal.(string)
		if !exists || !ok || strings.ToLower(strings.TrimSpace(roleStr)) != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin role required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// parseToken validates and extracts claims
func parseToken(tokenStr, expectedType string) (jwt.MapClaims, error) {
	if len(secretKey) == 0 {
		return nil, fmt.Errorf("JWT config not initialized")
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
