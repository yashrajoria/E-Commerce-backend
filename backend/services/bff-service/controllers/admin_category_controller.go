package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"bff-service/utils"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AdminCategoryController struct {
	logger     *zap.Logger
	httpClient *http.Client
	baseURL    string
}

type adminCategoryUpsertRequest struct {
	Name        string   `json:"name" binding:"required"`
	ParentNames []string `json:"parent_names,omitempty"`
	Image       string   `json:"image,omitempty"`
	IsActive    *bool    `json:"is_active,omitempty"`
}

func NewAdminCategoryController(logger *zap.Logger, httpClient *http.Client) *AdminCategoryController {
	baseURL := os.Getenv("PRODUCT_SERVICE_URL")
	if baseURL == "" {
		baseURL = "http://product-service:8082"
	}

	return &AdminCategoryController{
		logger:     logger,
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

func (ac *AdminCategoryController) ListCategories(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "ListCategories"))

	page, pageSize := parsePagination(c)
	downstreamURL := withPagination(ac.baseURL+"/categories", page, pageSize)

	ac.logger.Debug("forward admin request", zap.String("handler", "ListCategories"), zap.String("method", http.MethodGet), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardGet(c.Request.Context(), ac.httpClient, downstreamURL, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "ListCategories"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "ListCategories"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminCategoryController) CreateCategory(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "CreateCategory"))

	bodyBytes, ok := ac.bindAndValidateBody(c, "CreateCategory")
	if !ok {
		return
	}

	downstreamURL := ac.baseURL + "/categories"
	ac.logger.Debug("forward admin request", zap.String("handler", "CreateCategory"), zap.String("method", http.MethodPost), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardPost(c.Request.Context(), ac.httpClient, downstreamURL, bodyBytes, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "CreateCategory"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "CreateCategory"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminCategoryController) UpdateCategory(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "UpdateCategory"))

	categoryID := strings.TrimSpace(c.Param("id"))
	if !isValidPathID(categoryID) {
		ac.logger.Warn("invalid category id", zap.String("handler", "UpdateCategory"), zap.String("category_id", categoryID))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return
	}

	bodyBytes, ok := ac.bindAndValidateBody(c, "UpdateCategory")
	if !ok {
		return
	}

	downstreamURL := fmt.Sprintf("%s/categories/%s", ac.baseURL, url.PathEscape(categoryID))
	ac.logger.Debug("forward admin request", zap.String("handler", "UpdateCategory"), zap.String("method", http.MethodPut), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardPut(c.Request.Context(), ac.httpClient, downstreamURL, bodyBytes, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "UpdateCategory"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "UpdateCategory"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminCategoryController) DeleteCategory(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "DeleteCategory"))

	categoryID := strings.TrimSpace(c.Param("id"))
	if !isValidPathID(categoryID) {
		ac.logger.Warn("invalid category id", zap.String("handler", "DeleteCategory"), zap.String("category_id", categoryID))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return
	}

	downstreamURL := fmt.Sprintf("%s/categories/%s", ac.baseURL, url.PathEscape(categoryID))
	ac.logger.Debug("forward admin request", zap.String("handler", "DeleteCategory"), zap.String("method", http.MethodDelete), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardDelete(c.Request.Context(), ac.httpClient, downstreamURL, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "DeleteCategory"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "DeleteCategory"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminCategoryController) bindAndValidateBody(c *gin.Context, handler string) ([]byte, bool) {
	var payload adminCategoryUpsertRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		ac.logger.Warn("invalid request body", zap.String("handler", handler), zap.Error(err))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return nil, false
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		ac.logger.Error("failed to encode request body", zap.String("handler", handler), zap.Error(err))
		utils.ErrorResponse(c, http.StatusInternalServerError, "unexpected error")
		return nil, false
	}

	return bodyBytes, true
}

func (ac *AdminCategoryController) handleForwardResponse(c *gin.Context, statusCode int, responseBody []byte) {
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
