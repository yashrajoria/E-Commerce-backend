package middlewares

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const CorrelationIDHeader = "X-Correlation-ID"

// CorrelationIDMiddleware ensures every request has a unique correlation ID
// for distributed tracing across microservices.
func CorrelationIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		corID := c.GetHeader(CorrelationIDHeader)
		if corID == "" {
			corID = uuid.New().String()
		}

		// Set in Gin context for internal use
		c.Set("CorrelationID", corID)

		// Set in request header so it gets forwarded to downstream services
		c.Request.Header.Set(CorrelationIDHeader, corID)

		// Set in response header so clients can report it in case of errors
		c.Writer.Header().Set(CorrelationIDHeader, corID)

		c.Next()
	}
}
