package middleware

import (
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// SecurityHeaders adds security-related headers to all responses
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent clickjacking
		c.Header("X-Frame-Options", "DENY")

		// Enable XSS protection in browsers
		c.Header("X-XSS-Protection", "1; mode=block")

		// Prevent MIME type sniffing
		c.Header("X-Content-Type-Options", "nosniff")

		// Strict Transport Security
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")

		// Content Security Policy
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self'")

		// Referrer Policy
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Permissions Policy (formerly Feature-Policy)
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		// Cache Control
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")

		c.Next()
	}
}

// RateLimiter stores rate limiters for different IPs
type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type RateLimiter struct {
	ips   map[string]*limiterEntry
	mu    *sync.RWMutex
	rate  rate.Limit
	burst int
	ttl   time.Duration
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(r rate.Limit, b int, ttl time.Duration) *RateLimiter {
	rl := &RateLimiter{
		ips:   make(map[string]*limiterEntry),
		mu:    &sync.RWMutex{},
		rate:  r,
		burst: b,
		ttl:   ttl,
	}

	// Periodic cleanup of stale entries to avoid unbounded map growth
	go func() {
		ticker := time.NewTicker(ttl)
		defer ticker.Stop()
		for range ticker.C {
			rl.mu.Lock()
			now := time.Now()
			for ip, e := range rl.ips {
				if now.Sub(e.lastSeen) > rl.ttl {
					delete(rl.ips, ip)
				}
			}
			rl.mu.Unlock()
		}
	}()

	return rl
}

// GetLimiter returns the rate limiter for the given IP
func (rl *RateLimiter) GetLimiter(ip string) *rate.Limiter {
	rl.mu.RLock()
	entry, exists := rl.ips[ip]
	rl.mu.RUnlock()
	if exists {
		// update lastSeen
		rl.mu.Lock()
		entry.lastSeen = time.Now()
		rl.mu.Unlock()
		return entry.limiter
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()
	// double-check in case another goroutine created it
	entry, exists = rl.ips[ip]
	if !exists {
		entry = &limiterEntry{
			limiter:  rate.NewLimiter(rl.rate, rl.burst),
			lastSeen: time.Now(),
		}
		rl.ips[ip] = entry
	} else {
		entry.lastSeen = time.Now()
	}
	return entry.limiter
}

// RateLimitMiddleware creates a rate limiting middleware
func RateLimitMiddleware() gin.HandlerFunc {
	// Create a rate limiter: 100 requests per minute with burst of 50
	// Use rate.Every to express "per minute" correctly
	perMinute := rate.Every(time.Minute / 100)
	limiter := NewRateLimiter(perMinute, 50, time.Minute*5)

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

// CORSMiddleware creates a CORS middleware
func CORSMiddleware() gin.HandlerFunc {
	// Build allowlist from env var or fallbacks
	allowedEnv := os.Getenv("ALLOWED_ORIGINS")
	var allowed []string
	if allowedEnv == "*" {
		allowed = []string{"*"}
	} else if allowedEnv != "" {
		for _, o := range strings.Split(allowedEnv, ",") {
			allowed = append(allowed, strings.TrimSpace(strings.TrimSuffix(o, "/")))
		}
	} else {
		allowed = []string{"http://localhost:3000", "http://localhost:3001", "https://shopswift-storefront.vercel.app", "https://shopswift-admin.vercel.app"}
	}

	allowAll := len(allowed) == 1 && allowed[0] == "*"

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			c.Next()
			return
		}

		normalized := strings.TrimSuffix(origin, "/")
		allowedOrigin := ""
		if allowAll {
			allowedOrigin = origin
		} else {
			for _, a := range allowed {
				if a == normalized {
					allowedOrigin = origin
					break
				}
			}
		}

		if allowedOrigin == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Origin not allowed"})
			return
		}

		c.Header("Access-Control-Allow-Origin", allowedOrigin)
		c.Header("Vary", "Origin")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		c.Header("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	}
}
