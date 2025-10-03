package utils

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/api-gateway/logger"

	"go.uber.org/zap"
)

type ForwardOptions struct {
	TargetBase  string
	StripPrefix string
}

func ForwardRequest(c *gin.Context, opts ForwardOptions) {
	// Handle preflight OPTIONS requests immediately
	if c.Request.Method == "OPTIONS" {
		c.Header("Access-Control-Allow-Origin", "http://localhost:3000")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "43200")
		c.AbortWithStatus(http.StatusNoContent)
		return
	}

	targetPath := c.Param("any")
	if opts.StripPrefix != "" && strings.HasPrefix(targetPath, opts.StripPrefix) {
		targetPath = strings.TrimPrefix(targetPath, opts.StripPrefix)
	}

	targetURL := opts.TargetBase + targetPath
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	logger.Log.Info("🔁 Forwarding request", zap.String("method", c.Request.Method), zap.String("url", targetURL))

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

	// Inject user claims headers for downstream
	if userID, exists := c.Get("user_id"); exists {
		req.Header.Set("X-User-ID", userID.(string))
	}
	if email, exists := c.Get("email"); exists {
		req.Header.Set("X-User-Email", email.(string))
	}
	if role, exists := c.Get("role"); exists {
		req.Header.Set("X-User-Role", role.(string))
	}

	client := http.DefaultClient
	resp, err := client.Do(req)
	if err != nil {
		logger.Log.Error("❌ Failed to forward request", zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": "service unreachable"})
		return
	}
	defer resp.Body.Close()

	// Set CORS headers FIRST, before copying other headers
	c.Header("Access-Control-Allow-Origin", "http://localhost:3000")
	c.Header("Access-Control-Allow-Credentials", "true")
	c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")

	// Copy response headers (skip CORS from downstream)
	for k, v := range resp.Header {
		if strings.HasPrefix(strings.ToLower(k), "access-control-") {
			continue // Skip CORS headers, handled by gateway
		}
		c.Header(k, strings.Join(v, ","))
	}

	// Set status AFTER all headers are set
	c.Status(resp.StatusCode)

	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		logger.Log.Error("❌ Failed to copy response body", zap.Error(err))
	}
}
