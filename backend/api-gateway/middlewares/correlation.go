package middlewares

import (
	"time"

	"api-gateway/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	CorrelationIDHeader = "X-Correlation-ID"
	RequestIDHeader     = "X-Request-ID"
)

// RequestIDMiddleware ensures every request has X-Request-ID (and mirrors it to X-Correlation-ID when missing).
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader(RequestIDHeader)
		if reqID == "" {
			reqID = c.GetHeader(CorrelationIDHeader)
		}
		if reqID == "" {
			reqID = uuid.New().String()
		}

		c.Set("RequestID", reqID)
		c.Set("CorrelationID", reqID)
		c.Request.Header.Set(RequestIDHeader, reqID)
		c.Request.Header.Set(CorrelationIDHeader, reqID)
		c.Writer.Header().Set(RequestIDHeader, reqID)
		c.Writer.Header().Set(CorrelationIDHeader, reqID)
		c.Next()
	}
}

// CorrelationIDMiddleware is kept for compatibility; prefer RequestIDMiddleware.
func CorrelationIDMiddleware() gin.HandlerFunc {
	return RequestIDMiddleware()
}

// StructuredRequestLogger logs method/path/status/latency with request ID.
func StructuredRequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method
		c.Next()
		reqID, _ := c.Get("RequestID")
		fields := []zap.Field{
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
			zap.Int("body_size", c.Writer.Size()),
			zap.Any("request_id", reqID),
		}
		status := c.Writer.Status()
		switch {
		case status >= 500:
			logger.Log.Error("http_request", fields...)
		case status >= 400:
			logger.Log.Warn("http_request", fields...)
		default:
			logger.Log.Info("http_request", fields...)
		}
	}
}
