package middleware

import (
	"api-gateway/logger"
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

var rateLimitScript = redis.NewScript(`
local c = redis.call('INCR', KEYS[1])
if c == 1 or redis.call('PTTL', KEYS[1]) < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return c`)

// rateLimiter is a helper function that implements a Redis fixed-window counter
func rateLimiter(client *redis.Client, prefix string, limit int, duration time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Do not rate-limit CORS preflight requests.
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		ip := c.ClientIP()
		key := fmt.Sprintf("rate:%s:%s", prefix, ip)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		// Fixed window counter. INCR + expiry run atomically in one script, and the
		// TTL is (re)set if a key ever lacks one, so a crash can't leave a
		// permanent ban. Failure (including Redis being unreachable) fails open —
		// availability is prioritized over strict rate limiting here.
		count, err := rateLimitScript.Run(ctx, client, []string{key}, duration.Milliseconds()).Int64()
		if err != nil {
			logger.Log.Warn("rate limiter: redis unavailable, failing open",
				zap.String("ip", ip),
				zap.String("key", key),
				zap.Error(err),
			)
			c.Next()
			return
		}

		// Allow request if under limit
		if count <= int64(limit) {
			logger.Log.Info("rate limit check passed",
				zap.String("ip", ip),
				zap.String("endpoint", c.Request.URL.Path),
				zap.Int64("current_count", count),
				zap.Int("limit", limit),
			)
			c.Next()
			return
		}

		// Limit Exceeded
		logger.Log.Warn("rate limit exceeded",
			zap.String("ip", ip),
			zap.String("endpoint", c.Request.URL.Path),
			zap.Int("limit", limit),
		)

		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":       "too many requests",
			"retry_after": int(duration.Seconds()),
		})
		c.Abort()
	}
}

func getEnvInt(name string, defaultValue int) int {
	v := os.Getenv(name)
	if v == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(v)
	if err != nil || parsed <= 0 {
		return defaultValue
	}
	return parsed
}

// GlobalRateLimiter applies a 100 requests per minute limit across all routes
func GlobalRateLimiter(client *redis.Client) gin.HandlerFunc {
	limit := getEnvInt("RATE_LIMIT_GLOBAL", 100)
	windowSeconds := getEnvInt("RATE_LIMIT_WINDOW_SECONDS", 60)
	return rateLimiter(client, "global", limit, time.Duration(windowSeconds)*time.Second)
}

// StrictRateLimiter applies a 5 requests per minute limit for sensitive endpoints like auth
func StrictRateLimiter(client *redis.Client) gin.HandlerFunc {
	limit := getEnvInt("RATE_LIMIT_STRICT", 5)
	windowSeconds := getEnvInt("RATE_LIMIT_WINDOW_SECONDS", 60)
	return rateLimiter(client, "strict", limit, time.Duration(windowSeconds)*time.Second)
}
