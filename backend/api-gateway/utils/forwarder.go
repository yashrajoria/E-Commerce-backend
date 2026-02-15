package utils

import (
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"api-gateway/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type ForwardOptions struct {
	TargetBase  string
	StripPrefix string
}

func ForwardRequest(c *gin.Context, opts ForwardOptions) {
	// Build the path suffix to append to TargetBase.
	// 1. Try the wildcard param (*any) used by most routes.
	// 2. Fall back to deriving it from the full request path by stripping
	//    the gateway-level prefix that mirrors the TargetBase tail segment.
	//    e.g. TargetBase="http://inventory-service:8084/inventory"
	//         request path="/inventory/check"  →  suffix="/check"
	targetPath := ""
	if any := c.Param("any"); any != "" {
		targetPath = any
	} else {
		// Derive the gateway prefix from the last path segment of TargetBase.
		// For "http://host:port/inventory" the prefix is "/inventory".
		basePath := opts.TargetBase
		if idx := strings.Index(basePath, "://"); idx != -1 {
			// strip scheme+host → "/inventory"
			after := basePath[idx+3:]
			if si := strings.Index(after, "/"); si != -1 {
				basePath = after[si:]
			} else {
				basePath = ""
			}
		}
		reqPath := c.Request.URL.Path
		if basePath != "" && strings.HasPrefix(reqPath, basePath) {
			targetPath = strings.TrimPrefix(reqPath, basePath)
		}
		// Also check for named params (e.g. :productId) and append them
		if productId := c.Param("productId"); productId != "" && targetPath == "" {
			targetPath = "/" + productId
		}
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

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
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

		for _, val := range v {
			if strings.ToLower(k) == "set-cookie" {
				// Log Set-Cookie values coming from downstream for visibility
				log.Printf("[GATEWAY][FORWARD] downstream Set-Cookie: %s", val)
			}
			c.Writer.Header().Add(k, val)
		}
	}

	// Set status AFTER all headers are set
	c.Status(resp.StatusCode)

	// Copy response body
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		logger.Log.Error("❌ Failed to copy response body", zap.Error(err))
	}
}
