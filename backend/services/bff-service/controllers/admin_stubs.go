package controllers

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"

	"bff-service/utils"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/common/internalauth"
	"go.uber.org/zap"
)

// AdminProductController handles presigned URL proxying for product images.
// The api-gateway forwards /bff/admin/products/presign and
// /bff/admin/products/:id/images/presign directly to the bff-service so that
// the BFF can forward them to the product-service with the correct headers.
// All other product CRUD routes are proxied directly by the api-gateway and
// never reach this controller.
type AdminProductController struct {
	logger     *zap.Logger
	httpClient *http.Client
	baseURL    string
}

func NewAdminProductController(logger *zap.Logger, client *http.Client) *AdminProductController {
	return &AdminProductController{logger: logger, httpClient: client, baseURL: "http://product-service:8082"}
}

// bindUpdateProductBody validates an update payload allowing images-only updates.
func (apc *AdminProductController) bindUpdateProductBody(c *gin.Context) ([]byte, bool) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, false
	}
	// Empty body → reject
	if len(body) == 0 {
		return nil, false
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false
	}
	// Accept only if payload has exactly one key "images" (images-only)
	if len(payload) == 1 {
		if _, ok := payload["images"]; ok {
			return body, true
		}
	}
	return nil, false
}

// GetPresignUpload proxies GET /bff/admin/products/presign → product-service.
func (apc *AdminProductController) GetPresignUpload(c *gin.Context) {
	url := apc.baseURL + "/products/presign"
	if q := c.Request.URL.RawQuery; q != "" {
		url += "?" + q
	}
	resp, err := apc.httpClient.Get(url)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "downstream unavailable"})
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	c.Header("Content-Type", resp.Header.Get("Content-Type"))
	c.Status(resp.StatusCode)
	c.Writer.Write(data)
}

// PostProductImagePresign proxies POST /bff/admin/products/:id/images/presign → product-service.
func (apc *AdminProductController) PostProductImagePresign(c *gin.Context) {
	id := c.Param("id")
	url := apc.baseURL + "/products/" + id + "/images/presign"
	if q := c.Request.URL.RawQuery; q != "" {
		url += "?" + q
	}
	req, err := http.NewRequest(http.MethodPost, url, c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
		return
	}
	req.Header = c.Request.Header.Clone()
	for k := range req.Header {
		if utils.IsHopByHopHeader(k) {
			req.Header.Del(k)
		}
	}
	// AdminAuthMiddleware already validated the internal mesh token + admin role
	// upstream, but don't relay X-User-ID/X-User-Role on trust alone — only
	// forward them if this request itself carries a valid mesh token, so this
	// handler stays safe even if reached by a future caller that skips that middleware.
	expected := internalauth.Token()
	if expected == "" || subtle.ConstantTimeCompare([]byte(req.Header.Get(internalauth.Header)), []byte(expected)) != 1 {
		req.Header.Del("X-User-ID")
		req.Header.Del("X-User-Role")
	}
	resp, err := apc.httpClient.Do(req)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "downstream unavailable"})
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	c.Header("Content-Type", resp.Header.Get("Content-Type"))
	c.Status(resp.StatusCode)
	c.Writer.Write(data)
}
