package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"bff-service/utils"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AdminProductController struct {
	logger     *zap.Logger
	httpClient *http.Client
	baseURL    string
}

type adminProductCreateRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description" binding:"required"`
	Brand       string   `json:"brand" binding:"required"`
	SKU         string   `json:"sku" binding:"required"`
	Price       float64  `json:"price" binding:"required,gt=0"`
	Quantity    int      `json:"quantity" binding:"required,gte=0"`
	IsFeatured  bool     `json:"is_featured"`
	Categories  []string `json:"category" binding:"required,min=1,dive,required"`
}

type adminProductUpdateRequest struct {
	Name        *string   `json:"name,omitempty"`
	Description *string   `json:"description,omitempty"`
	Brand       *string   `json:"brand,omitempty"`
	SKU         *string   `json:"sku,omitempty"`
	Price       *float64  `json:"price,omitempty" binding:"omitempty,gt=0"`
	Quantity    *int      `json:"quantity,omitempty" binding:"omitempty,gte=0"`
	IsFeatured  *bool     `json:"is_featured,omitempty"`
	Categories  *[]string `json:"category,omitempty" binding:"omitempty,min=1,dive,required"`
	Images      *[]string `json:"images,omitempty"`
}

func NewAdminProductController(logger *zap.Logger, httpClient *http.Client) *AdminProductController {
	baseURL := os.Getenv("PRODUCT_SERVICE_URL")
	if baseURL == "" {
		baseURL = "http://product-service:8082"
	}

	return &AdminProductController{
		logger:     logger,
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

func (ac *AdminProductController) ListProducts(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "ListProducts"))

	page, pageSize := parsePagination(c)
	downstreamURL := withPagination(ac.baseURL+"/products", page, pageSize)

	ac.logger.Debug("forward admin request", zap.String("handler", "ListProducts"), zap.String("method", http.MethodGet), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardGet(c.Request.Context(), ac.httpClient, downstreamURL, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "ListProducts"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "ListProducts"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminProductController) CreateProduct(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "CreateProduct"))

	bodyBytes, ok := ac.bindCreateProductBody(c)
	if !ok {
		return
	}

	downstreamURL := ac.baseURL + "/products"
	ac.logger.Debug("forward admin request", zap.String("handler", "CreateProduct"), zap.String("method", http.MethodPost), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardPost(c.Request.Context(), ac.httpClient, downstreamURL, bodyBytes, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "CreateProduct"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "CreateProduct"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminProductController) UpdateProduct(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "UpdateProduct"))

	productID := strings.TrimSpace(c.Param("id"))
	if !isValidPathID(productID) {
		ac.logger.Warn("invalid product id", zap.String("handler", "UpdateProduct"), zap.String("product_id", productID))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return
	}

	bodyBytes, ok := ac.bindUpdateProductBody(c)
	if !ok {
		return
	}

	downstreamURL := fmt.Sprintf("%s/products/%s", ac.baseURL, url.PathEscape(productID))
	ac.logger.Debug("forward admin request", zap.String("handler", "UpdateProduct"), zap.String("method", http.MethodPut), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardPut(c.Request.Context(), ac.httpClient, downstreamURL, bodyBytes, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "UpdateProduct"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "UpdateProduct"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminProductController) DeleteProduct(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "DeleteProduct"))

	productID := strings.TrimSpace(c.Param("id"))
	if !isValidPathID(productID) {
		ac.logger.Warn("invalid product id", zap.String("handler", "DeleteProduct"), zap.String("product_id", productID))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return
	}

	downstreamURL := fmt.Sprintf("%s/products/%s", ac.baseURL, url.PathEscape(productID))
	ac.logger.Debug("forward admin request", zap.String("handler", "DeleteProduct"), zap.String("method", http.MethodDelete), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardDelete(c.Request.Context(), ac.httpClient, downstreamURL, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "DeleteProduct"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "DeleteProduct"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminProductController) GetPresignUpload(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "GetPresignUpload"))

	downstreamURL := ac.withQuery(ac.baseURL+"/products/presign", c.Request.URL.RawQuery)
	ac.logger.Debug("forward admin request", zap.String("handler", "GetPresignUpload"), zap.String("method", http.MethodGet), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardGet(c.Request.Context(), ac.httpClient, downstreamURL, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "GetPresignUpload"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "GetPresignUpload"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminProductController) PostProductImagePresign(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "PostProductImagePresign"))

	productID := strings.TrimSpace(c.Param("id"))
	if !isValidPathID(productID) {
		ac.logger.Warn("invalid product id", zap.String("handler", "PostProductImagePresign"), zap.String("product_id", productID))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return
	}

	downstreamURL := ac.withQuery(fmt.Sprintf("%s/products/%s/images/presign", ac.baseURL, url.PathEscape(productID)), c.Request.URL.RawQuery)
	ac.logger.Debug("forward admin request", zap.String("handler", "PostProductImagePresign"), zap.String("method", http.MethodPost), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardPost(c.Request.Context(), ac.httpClient, downstreamURL, nil, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "PostProductImagePresign"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "PostProductImagePresign"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminProductController) bindCreateProductBody(c *gin.Context) ([]byte, bool) {
	var payload adminProductCreateRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		ac.logger.Warn("invalid request body", zap.String("handler", "CreateProduct"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return nil, false
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		ac.logger.Error("failed to encode request body", zap.String("handler", "CreateProduct"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusInternalServerError, "unexpected error")
		return nil, false
	}

	return bodyBytes, true
}

func (ac *AdminProductController) bindUpdateProductBody(c *gin.Context) ([]byte, bool) {
	var payload adminProductUpdateRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		ac.logger.Warn("invalid request body", zap.String("handler", "UpdateProduct"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return nil, false
	}

	if payload.Name == nil && payload.Description == nil && payload.Brand == nil &&
		payload.SKU == nil && payload.Price == nil && payload.Quantity == nil &&
		payload.IsFeatured == nil && payload.Categories == nil && payload.Images == nil {
		ac.logger.Warn("empty request body", zap.String("handler", "UpdateProduct"))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return nil, false
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		ac.logger.Error("failed to encode request body", zap.String("handler", "UpdateProduct"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusInternalServerError, "unexpected error")
		return nil, false
	}

	return bodyBytes, true
}

func (ac *AdminProductController) handleForwardResponse(c *gin.Context, statusCode int, responseBody []byte) {
	if statusCode == http.StatusNotFound {
		utils.ErrorResponse(c, http.StatusNotFound, "not found")
		return
	}
	if statusCode >= http.StatusInternalServerError {
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	if statusCode >= http.StatusBadRequest {
		utils.ErrorResponse(c, statusCode, utils.MapDownstreamError(statusCode))
		return
	}

	data := decodeResponseBody(responseBody)
	if statusCode == http.StatusCreated {
		utils.CreatedResponse(c, data)
		return
	}
	utils.SuccessResponse(c, data, nil)
}

func parsePagination(c *gin.Context) (int, int) {
	page := 1
	if raw := c.DefaultQuery("page", "1"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			page = v
		}
	}

	pageSize := 20
	if raw := c.DefaultQuery("page_size", "20"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			if v > 100 {
				v = 100
			}
			pageSize = v
		}
	}

	return page, pageSize
}

func withPagination(rawURL string, page, pageSize int) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := parsed.Query()
	q.Set("page", strconv.Itoa(page))
	q.Set("page_size", strconv.Itoa(pageSize))
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

func isValidPathID(id string) bool {
	if id == "" {
		return false
	}
	if strings.ContainsAny(id, " \t\n\r") {
		return false
	}
	return true
}

func decodeResponseBody(responseBody []byte) interface{} {
	if len(responseBody) == 0 {
		return gin.H{}
	}

	var decoded interface{}
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return gin.H{"raw": string(responseBody)}
	}
	return decoded
}

func (ac *AdminProductController) withQuery(rawURL, rawQuery string) string {
	if rawQuery == "" {
		return rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	parsed.RawQuery = rawQuery
	return parsed.String()
}
