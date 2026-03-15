package controllers

import (
	"net/http"
	"os"
	"strings"

	"bff-service/utils"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AdminNotificationController struct {
	logger     *zap.Logger
	httpClient *http.Client
	baseURL    string
}

func NewAdminNotificationController(logger *zap.Logger, httpClient *http.Client) *AdminNotificationController {
	baseURL := os.Getenv("NOTIFICATION_SERVICE_URL")
	if baseURL == "" {
		baseURL = "http://notification-service:8092"
	}

	return &AdminNotificationController{
		logger:     logger,
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

func (ac *AdminNotificationController) ListNotifications(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "ListNotifications"))

	page, pageSize := parsePagination(c)
	downstreamURL := withPagination(ac.baseURL+"/notifications", page, pageSize)

	ac.logger.Debug("forward admin request", zap.String("handler", "ListNotifications"), zap.String("method", http.MethodGet), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardGet(c.Request.Context(), ac.httpClient, downstreamURL, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "ListNotifications"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "ListNotifications"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminNotificationController) NotificationLog(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "NotificationLog"))

	downstreamURL := ac.baseURL + "/notifications/log"
	ac.logger.Debug("forward admin request", zap.String("handler", "NotificationLog"), zap.String("method", http.MethodGet), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardGet(c.Request.Context(), ac.httpClient, downstreamURL, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "NotificationLog"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "NotificationLog"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminNotificationController) handleForwardResponse(c *gin.Context, statusCode int, responseBody []byte) {
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
