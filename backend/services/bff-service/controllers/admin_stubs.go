package controllers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
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
	req.Header = c.Request.Header
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
