package controllers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"bff-service/utils"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AdminOrderController struct {
	logger     *zap.Logger
	httpClient *http.Client
	baseURL    string
}

type adminOrderStatusUpdateRequest struct {
	Status string `json:"status" binding:"required"`
}

func NewAdminOrderController(logger *zap.Logger, httpClient *http.Client) *AdminOrderController {
	baseURL := os.Getenv("ORDER_SERVICE_URL")
	if baseURL == "" {
		baseURL = "http://order-service:8083"
	}

	return &AdminOrderController{
		logger:     logger,
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

func (ac *AdminOrderController) ListOrders(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "ListOrders"))

	page, pageSize := parsePagination(c)
	downstreamURL := withPagination(ac.baseURL+"/orders/admin/", page, pageSize)
	log.Println("ORDERS URL", downstreamURL)

	ac.logger.Debug("forward admin request", zap.String("handler", "ListOrders"), zap.String("method", http.MethodGet), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardGet(c.Request.Context(), ac.httpClient, downstreamURL, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "ListOrders"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "ListOrders"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminOrderController) GetOrderByID(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "GetOrderByID"))

	orderID := strings.TrimSpace(c.Param("id"))
	if !isValidPathID(orderID) {
		ac.logger.Warn("invalid order id", zap.String("handler", "GetOrderByID"), zap.String("order_id", orderID))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return
	}

	downstreamURL := fmt.Sprintf("%s/orders/%s", ac.baseURL, url.PathEscape(orderID))
	ac.logger.Debug("forward admin request", zap.String("handler", "GetOrderByID"), zap.String("method", http.MethodGet), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardGet(c.Request.Context(), ac.httpClient, downstreamURL, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "GetOrderByID"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "GetOrderByID"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminOrderController) UpdateOrderStatus(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "UpdateOrderStatus"))

	orderID := strings.TrimSpace(c.Param("id"))
	if !isValidPathID(orderID) {
		ac.logger.Warn("invalid order id", zap.String("handler", "UpdateOrderStatus"), zap.String("order_id", orderID))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return
	}

	bodyBytes, ok := ac.bindAndValidateBody(c, "UpdateOrderStatus")
	if !ok {
		return
	}

	downstreamURL := fmt.Sprintf("%s/orders/%s/status", ac.baseURL, url.PathEscape(orderID))
	ac.logger.Debug("forward admin request", zap.String("handler", "UpdateOrderStatus"), zap.String("method", http.MethodPut), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardPut(c.Request.Context(), ac.httpClient, downstreamURL, bodyBytes, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "UpdateOrderStatus"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "UpdateOrderStatus"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminOrderController) bindAndValidateBody(c *gin.Context, handler string) ([]byte, bool) {
	var payload adminOrderStatusUpdateRequest
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

func (ac *AdminOrderController) handleForwardResponse(c *gin.Context, statusCode int, responseBody []byte) {
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
