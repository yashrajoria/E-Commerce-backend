package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// MetricsMiddleware creates a Gin middleware that tracks HTTP latency and status codes
func MetricsMiddleware(unused interface{}, serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)
		status := c.Writer.Status()

		if status >= 500 {
			zap.L().Error("HTTP request failed",
				zap.String("service", serviceName),
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
				zap.Int("status", status),
				zap.Duration("duration", duration),
			)
		}
	}
}

// statusCodeToRange converts status code to a range string (2xx, 3xx, 4xx, 5xx)
func statusCodeToRange(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return "2xx"
	case statusCode >= 300 && statusCode < 400:
		return "3xx"
	case statusCode >= 400 && statusCode < 500:
		return "4xx"
	case statusCode >= 500:
		return "5xx"
	default:
		return "unknown"
	}
}
