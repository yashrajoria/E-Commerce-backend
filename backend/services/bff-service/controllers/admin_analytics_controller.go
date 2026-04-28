package controllers

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"bff-service/utils"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AdminAnalyticsController struct {
	logger           *zap.Logger
	httpClient       *http.Client
	orderServiceURL  string
	userServiceURL   string
	inventoryService string
}

func NewAdminAnalyticsController(logger *zap.Logger, httpClient *http.Client) *AdminAnalyticsController {
	orderURL := os.Getenv("ORDER_SERVICE_URL")
	if orderURL == "" {
		orderURL = "http://order-service:8083"
	}

	userURL := os.Getenv("USER_SERVICE_URL")
	if userURL == "" {
		userURL = "http://user-service:8085"
	}

	inventoryURL := os.Getenv("INVENTORY_SERVICE_URL")
	if inventoryURL == "" {
		inventoryURL = "http://inventory-service:8084"
	}

	return &AdminAnalyticsController{
		logger:           logger,
		httpClient:       httpClient,
		orderServiceURL:  strings.TrimRight(orderURL, "/"),
		userServiceURL:   strings.TrimRight(userURL, "/"),
		inventoryService: strings.TrimRight(inventoryURL, "/"),
	}
}

func (ac *AdminAnalyticsController) SalesReport(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "SalesReport"))

	ac.forwardReport(c, "SalesReport", ac.orderServiceURL+"/orders/admin/stats")
}

func (ac *AdminAnalyticsController) UsersReport(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "UsersReport"))

	ac.forwardReport(c, "UsersReport", ac.userServiceURL+"/users")
}

func (ac *AdminAnalyticsController) InventoryReport(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "InventoryReport"))

	ac.forwardReport(c, "InventoryReport", ac.inventoryService+"/inventory")
}

func (ac *AdminAnalyticsController) forwardReport(c *gin.Context, handler, downstreamURL string) {
	ac.logger.Debug("forward admin request", zap.String("handler", handler), zap.String("method", http.MethodGet), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardGet(c.Request.Context(), ac.httpClient, downstreamURL, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", handler), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", handler), zap.Int("status", statusCode))

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

	utils.SuccessResponse(c, decodeResponseBody(body), nil)
}

func decodeResponseBody(body []byte) interface{} {
	str := strings.TrimSpace(string(body))
	if str == "" {
		return gin.H{}
	}
	var v interface{}
	if err := json.Unmarshal(body, &v); err != nil {
		return gin.H{"raw": str}
	}
	return v
}
