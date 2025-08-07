// package utils

// import (
// 	"io"
// 	"net/http"
// 	"strings"

// 	"github.com/gin-gonic/gin"
// 	"github.com/yashrajoria/api-gateway/logger"
// 	"go.uber.org/zap"
// )

// type ForwardOptions struct {
// 	TargetBase  string
// 	StripPrefix string
// }

// func ForwardRequest(c *gin.Context, opts ForwardOptions) {
// 	targetPath := c.Param("any")
// 	if opts.StripPrefix != "" && strings.HasPrefix(targetPath, opts.StripPrefix) {
// 		targetPath = strings.TrimPrefix(targetPath, opts.StripPrefix)
// 	}
// 	targetURL := opts.TargetBase + targetPath

// 	if c.Request.URL.RawQuery != "" {
// 		targetURL += "?" + c.Request.URL.RawQuery
// 	}

// 	logger.Log.Info("🔁 Forwarding request", zap.String("method", c.Request.Method), zap.String("url", targetURL))

// 	req, err := http.NewRequest(c.Request.Method, targetURL, c.Request.Body)
// 	if err != nil {
// 		logger.Log.Error("❌ Failed to create forward request", zap.Error(err))
// 		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
// 		return
// 	}

// 	for k, v := range c.Request.Header {
// 		req.Header[k] = v
// 	}

// 	resp, err := http.DefaultClient.Do(req)
// 	if err != nil {
// 		logger.Log.Error("❌ Failed to forward request", zap.Error(err))
// 		c.JSON(http.StatusBadGateway, gin.H{"error": "service unreachable"})
// 		return
// 	}
// 	defer resp.Body.Close()

// 	for k, v := range resp.Header {
// 		c.Header(k, strings.Join(v, ","))
// 	}

// 	c.Status(resp.StatusCode)
// 	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
// 		logger.Log.Error("❌ Failed to copy response body", zap.Error(err))
// 	}
// }


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
	targetPath := c.Param("any")

	// Optionally remove prefix from path
	if opts.StripPrefix != "" && strings.HasPrefix(targetPath, opts.StripPrefix) {
		targetPath = strings.TrimPrefix(targetPath, opts.StripPrefix)
	}

	// Construct final target URL
	targetURL := opts.TargetBase + targetPath
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	logger.Log.Info("🔁 Forwarding request", zap.String("method", c.Request.Method), zap.String("url", targetURL))

	// Create new request
	req, err := http.NewRequest(c.Request.Method, targetURL, c.Request.Body)
	if err != nil {
		logger.Log.Error("❌ Failed to create forward request", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
		return
	}

	// Copy original request headers
	for k, v := range c.Request.Header {
		req.Header[k] = v
	}

	// Forward to backend
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Log.Error("❌ Failed to forward request", zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": "service unreachable"})
		return
	}
	defer resp.Body.Close()

	// ✅ Copy only non-CORS headers (let Gin CORS middleware handle Access-Control-* headers)
	for k, v := range resp.Header {
		if strings.HasPrefix(strings.ToLower(k), "access-control-") {
			continue
		}
		c.Header(k, strings.Join(v, ","))
	}

	// Write status and body
	c.Status(resp.StatusCode)
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		logger.Log.Error("❌ Failed to copy response body", zap.Error(err))
	}
}
