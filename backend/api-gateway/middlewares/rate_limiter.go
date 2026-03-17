package middlewares

import (
	"api-gateway/logger"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// rateLimiter is a helper function that implements the Redis sliding window pattern
func rateLimiter(client *redis.Client, prefix string, limit int, duration time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := fmt.Sprintf("rate:%s:%s", prefix, ip)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Attempt to ping Redis to ensure availability before proceeding
		if err := client.Ping(ctx).Err(); err != nil {
			logger.Log.Warn("rate limiter: redis unavailable, failing open",
				zap.String("ip", ip),
				zap.String("key", key),
				zap.Error(err),
			)
			c.Next()
			return
		}

		// Sliding window pattern:
		// 1. Increment the counter
		// 2. Set expiry if this is the first hit
		pipe := client.TxPipeline()
		incr := pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, duration)

		if _, err := pipe.Exec(ctx); err != nil {
			logger.Log.Warn("rate limiter: pipeline execution failed, failing open", zap.Error(err))
			c.Next()
			return
		}

		// Wait for the pipeline result
		count := incr.Val()

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

// GlobalRateLimiter applies a 100 requests per minute limit across all routes
func GlobalRateLimiter(client *redis.Client) gin.HandlerFunc {
	return rateLimiter(client, "global", 100, time.Minute)
}

// StrictRateLimiter applies a 5 requests per minute limit for sensitive endpoints like auth
func StrictRateLimiter(client *redis.Client) gin.HandlerFunc {
	return rateLimiter(client, "strict", 5, time.Minute)
}
