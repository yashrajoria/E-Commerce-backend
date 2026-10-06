package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RequestLogger returns a Gin middleware that emits a structured JSON log line
// for every HTTP request. Because the logger is typically tee'd to both console
// and a CloudWatch Logs writer, every request automatically appears in
// CloudWatch as well.
//
// Usage:
//
//	router.Use(middleware.RequestLogger(logger))
func RequestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		method := c.Request.Method
		clientIP := c.ClientIP()
		userAgent := c.Request.UserAgent()

		// Process request
		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		bodySize := c.Writer.Size()

		fields := []zap.Field{
			zap.String("method", method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("client_ip", clientIP),
			zap.String("user_agent", userAgent),
			zap.Int("body_size", bodySize),
		}

		// Extract Correlation ID and Request ID for distributed tracing
		correlationID := c.GetHeader("X-Correlation-ID")
		requestID := c.GetHeader("X-Request-Id")
		if requestID == "" {
			requestID = c.GetHeader("X-Request-ID")
		}

		// Also check Gin context if set by local middleware
		if rid, ok := c.Get("request_id"); ok && rid != "" {
			if s, ok := rid.(string); ok {
				requestID = s
			}
		}
		if cid, ok := c.Get("CorrelationID"); ok && cid != "" {
			if s, ok := cid.(string); ok {
				correlationID = s
			}
		}

		if correlationID != "" {
			fields = append(fields, zap.String("correlation_id", correlationID))
		}
		if requestID != "" {
			fields = append(fields, zap.String("request_id", requestID))
		}

		switch {
		case status >= 500:
			logger.Error("http_request", fields...)
		case status >= 400:
			logger.Warn("http_request", fields...)
		default:
			logger.Info("http_request", fields...)
		}
	}
}
