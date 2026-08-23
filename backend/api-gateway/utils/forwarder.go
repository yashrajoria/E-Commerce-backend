package utils

import (
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"api-gateway/logger"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/common/internalauth"
	"go.uber.org/zap"
)

// securityHeaderKeys are set by the gateway's own SecurityHeaders middleware
// and must not be duplicated from a downstream service's response.
var securityHeaderKeys = map[string]bool{
	"x-frame-options":           true,
	"x-xss-protection":          true,
	"x-content-type-options":    true,
	"strict-transport-security": true,
	"content-security-policy":   true,
	"referrer-policy":           true,
	"permissions-policy":        true,
	"cache-control":             true,
	"pragma":                    true,
	"expires":                   true,
}

func isSecurityHeader(lowerKey string) bool {
	return securityHeaderKeys[lowerKey]
}

type ForwardOptions struct {
	TargetBase  string
	StripPrefix string
}

var forwardHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
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

	// Defense in depth: reject any ".." segments so a crafted wildcard path
	// (e.g. "/products/../../internal/admin") can't escape TargetBase.
	if targetPath != "" {
		cleaned := path.Clean("/" + targetPath)
		if cleaned == "/" {
			targetPath = ""
		} else {
			targetPath = cleaned
		}
	}

	targetURL := opts.TargetBase + targetPath
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	logger.Log.Info("🔁 Forwarding request",
		zap.String("method", c.Request.Method),
		zap.String("url", targetURL),
		zap.String("path", targetPath),
		zap.String("correlation_id", c.GetString("CorrelationID")),
	)

	req, err := http.NewRequest(c.Request.Method, targetURL, c.Request.Body)
	if err != nil {
		logger.Log.Error("❌ Failed to create forward request", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
		return
	}

	// Copy original headers, but never trust client-supplied identity claims.
	for k, v := range c.Request.Header {
		lower := strings.ToLower(k)
		if lower == "x-user-id" || lower == "x-user-email" || lower == "x-user-role" ||
			lower == "x-internal-service-token" {
			continue
		}
		req.Header[k] = v
	}
	req.Header.Del("X-User-ID")
	req.Header.Del("X-User-Email")
	req.Header.Del("X-User-Role")
	req.Header.Del(internalauth.Header)

	// Inject identity from JWT context only (set by JWTMiddleware on protected/admin routes).
	if userID, exists := c.Get("user_id"); exists {
		if uid, ok := userID.(string); ok && uid != "" {
			req.Header.Set("X-User-ID", uid)
			req.AddCookie(&http.Cookie{Name: "user_id", Value: uid, HttpOnly: true, Path: "/"})
		}
	}
	if email, exists := c.Get("email"); exists {
		if e, ok := email.(string); ok && e != "" {
			req.Header.Set("X-User-Email", e)
			req.AddCookie(&http.Cookie{Name: "user_email", Value: e, HttpOnly: true, Path: "/"})
		}
	}
	if role, exists := c.Get("role"); exists {
		if r, ok := role.(string); ok && r != "" {
			req.Header.Set("X-User-Role", r)
			req.AddCookie(&http.Cookie{Name: "user_role", Value: r, HttpOnly: true, Path: "/"})
		}
	}

	// Gateway is the trusted ingress — attach mesh token for internal-gated routes
	// (e.g. inventory /check) that frontend users reach via JWT on the gateway.
	internalauth.Apply(req)

	resp, err := forwardHTTPClient.Do(req)
	if err != nil {
		logger.Log.Error("❌ Failed to forward request", zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": "service unreachable"})
		return
	}
	defer resp.Body.Close()

	// Copy response headers (skip CORS/security/hop-by-hop headers from downstream)
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

		// Skip security headers — the gateway is the browser-facing edge and
		// already sets these itself (SecurityHeaders middleware). Forwarding
		// downstream's copy too would emit duplicate/conflicting header
		// values (browsers intersect multiple CSP headers, which can be
		// stricter than either service intended).
		if isSecurityHeader(lowerKey) {
			continue
		}

		for _, val := range v {

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
