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

type AdminInventoryController struct {
	logger     *zap.Logger
	httpClient *http.Client
	baseURL    string
}

type adminInventoryUpdateRequest struct {
	Available *int `json:"available,omitempty" binding:"omitempty,gte=0"`
	Threshold *int `json:"threshold,omitempty" binding:"omitempty,gte=0"`
}

func NewAdminInventoryController(logger *zap.Logger, httpClient *http.Client) *AdminInventoryController {
	baseURL := os.Getenv("INVENTORY_SERVICE_URL")
	if baseURL == "" {
		baseURL = "http://inventory-service:8084"
	}

	return &AdminInventoryController{
		logger:     logger,
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

func (ac *AdminInventoryController) ListInventory(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "ListInventory"))

	page, pageSize := parsePagination(c)
	downstreamURL := withPagination(ac.baseURL+"/inventory", page, pageSize)

	ac.logger.Debug("forward admin request", zap.String("handler", "ListInventory"), zap.String("method", http.MethodGet), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardGet(c.Request.Context(), ac.httpClient, downstreamURL, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "ListInventory"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "ListInventory"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminInventoryController) UpdateInventory(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "UpdateInventory"))

	productID := strings.TrimSpace(c.Param("product_id"))
	if !isValidPathID(productID) {
		ac.logger.Warn("invalid product id", zap.String("handler", "UpdateInventory"), zap.String("product_id", productID))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return
	}

	bodyBytes, ok := ac.bindAndValidateBody(c, "UpdateInventory")
	if !ok {
		return
	}

	downstreamURL := fmt.Sprintf("%s/inventory/%s", ac.baseURL, url.PathEscape(productID))
	ac.logger.Debug("forward admin request", zap.String("handler", "UpdateInventory"), zap.String("method", http.MethodPut), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardPut(c.Request.Context(), ac.httpClient, downstreamURL, bodyBytes, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "UpdateInventory"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "UpdateInventory"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminInventoryController) bindAndValidateBody(c *gin.Context, handler string) ([]byte, bool) {
	var payload adminInventoryUpdateRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		ac.logger.Warn("invalid request body", zap.String("handler", handler), zap.Error(err))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return nil, false
	}
	if payload.Available == nil && payload.Threshold == nil {
		ac.logger.Warn("empty request body", zap.String("handler", handler))
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

func (ac *AdminInventoryController) handleForwardResponse(c *gin.Context, statusCode int, responseBody []byte) {
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

	utils.SuccessResponse(c, decodeResponseBody(responseBody), nil)
}
