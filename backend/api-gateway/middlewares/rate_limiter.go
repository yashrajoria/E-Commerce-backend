package middlewares

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

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Fixed window counter: increment, then set the expiry only on the
		// first hit of a window so the TTL is not pushed forward on every
		// request. Failure (including Redis being unreachable) fails open —
		// availability is prioritized over strict rate limiting here.
		count, err := client.Incr(ctx, key).Result()
		if err != nil {
			logger.Log.Warn("rate limiter: redis unavailable, failing open",
				zap.String("ip", ip),
				zap.String("key", key),
				zap.Error(err),
			)
			c.Next()
			return
		}
		if count == 1 {
			if err := client.Expire(ctx, key, duration).Err(); err != nil {
				logger.Log.Warn("rate limiter: failed to set window expiry", zap.String("key", key), zap.Error(err))
			}
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
