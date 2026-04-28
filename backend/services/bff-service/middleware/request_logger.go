package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RequestLoggerMiddleware logs HTTP requests with request identity and timing.
func RequestLoggerMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()

		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}
		c.Writer.Header().Set("X-Request-ID", requestID)

		c.Next()

		latencyMs := time.Since(startedAt).Milliseconds()
		status := c.Writer.Status()
		userID := c.GetHeader("X-User-ID")
		userRole := c.GetHeader("X-User-Role")

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", status),
			zap.Int64("latency_ms", latencyMs),
			zap.String("user_id", userID),
			zap.String("user_role", userRole),
			zap.String("request_id", requestID),
		}

		switch {
		case status >= 500:
			logger.Error("admin_request", fields...)
		case status >= 400:
			logger.Warn("admin_request", fields...)
		default:
			logger.Info("admin_request", fields...)
		}
	}
}

func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// fallback keeps middleware non-blocking even if randomness source fails
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b)
}
