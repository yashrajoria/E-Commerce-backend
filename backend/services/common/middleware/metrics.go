package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	awspkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
)

// MetricsMiddleware creates a Gin middleware that tracks HTTP metrics
func MetricsMiddleware(metricsClient *awspkg.MetricsClient, serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if metricsClient == nil || !metricsClient.IsEnabled() {
			c.Next()
			return
		}

		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(start)
		statusCode := c.Writer.Status()

		// Build dimensions
		dimensions := map[string]string{
			"Service": serviceName,
			"Method":  method,
			"Path":    path,
			"Status":  statusCodeToRange(statusCode),
		}

		// Record metrics asynchronously to avoid blocking
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Record request count
			_ = metricsClient.RecordCount(ctx, awspkg.MetricHTTPRequests, dimensions)

			// Record latency
			_ = metricsClient.RecordLatency(ctx, awspkg.MetricHTTPLatency, duration, dimensions)

			// Record errors
			if statusCode >= 400 {
				_ = metricsClient.RecordCount(ctx, awspkg.MetricHTTPErrors, dimensions)

				if statusCode >= 400 && statusCode < 500 {
					_ = metricsClient.RecordCount(ctx, awspkg.MetricHTTP4xx, dimensions)
				} else if statusCode >= 500 {
					_ = metricsClient.RecordCount(ctx, awspkg.MetricHTTP5xx, dimensions)
				}
			}
		}()
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
