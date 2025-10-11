package utils

import (
	"io"
	"net/http"
	"strings"

	"api-gateway/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type ForwardOptions struct {
	TargetBase  string
	StripPrefix string
}

func ForwardRequest(c *gin.Context, opts ForwardOptions) {
	// Get the path - handle case where there's no wildcard parameter
	targetPath := ""
	if any := c.Param("any"); any != "" {
		targetPath = any
	}

	if opts.StripPrefix != "" && strings.HasPrefix(targetPath, opts.StripPrefix) {
		targetPath = strings.TrimPrefix(targetPath, opts.StripPrefix)
	}

	targetURL := opts.TargetBase + targetPath
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	logger.Log.Info("🔁 Forwarding request",
		zap.String("method", c.Request.Method),
		zap.String("url", targetURL),
		zap.String("path", targetPath),
	)

	req, err := http.NewRequest(c.Request.Method, targetURL, c.Request.Body)
	if err != nil {
		logger.Log.Error("❌ Failed to create forward request", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
		return
	}

	// Copy original headers
	for k, v := range c.Request.Header {
		req.Header[k] = v
	}

	// Inject user claims headers for downstream services
	if userID, exists := c.Get("user_id"); exists {
		if uid, ok := userID.(string); ok {
			req.Header.Set("X-User-ID", uid)
		}
	}
	if email, exists := c.Get("email"); exists {
		if e, ok := email.(string); ok {
			req.Header.Set("X-User-Email", e)
		}
	}
	if role, exists := c.Get("role"); exists {
		if r, ok := role.(string); ok {
			req.Header.Set("X-User-Role", r)
		}
	}

	client := http.DefaultClient
	resp, err := client.Do(req)
	if err != nil {
		logger.Log.Error("❌ Failed to forward request", zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": "service unreachable"})
		return
	}
	defer resp.Body.Close()

	// Copy response headers (skip CORS and hop-by-hop headers from downstream)
	for k, v := range resp.Header {
		lowerKey := strings.ToLower(k)

		// Skip CORS headers (handled by gateway middleware)
		if strings.HasPrefix(lowerKey, "access-control-") {
			continue
		}

		// Skip hop-by-hop headers (these are not meant to be forwarded)
		if lowerKey == "connection" || lowerKey == "keep-alive" ||
			lowerKey == "proxy-authenticate" || lowerKey == "proxy-authorization" ||
			lowerKey == "te" || lowerKey == "trailers" ||
			lowerKey == "transfer-encoding" || lowerKey == "upgrade" {
			continue
		}

		c.Header(k, strings.Join(v, ","))
	}

	// Set status AFTER all headers are set
	c.Status(resp.StatusCode)

	// Copy response body
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		logger.Log.Error("❌ Failed to copy response body", zap.Error(err))
	}
}
