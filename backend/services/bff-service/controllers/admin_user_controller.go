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

type AdminUserController struct {
	logger     *zap.Logger
	httpClient *http.Client
	baseURL    string
}

type adminUserRoleUpdateRequest struct {
	Role string `json:"role" binding:"required,oneof=admin user"`
}

func NewAdminUserController(logger *zap.Logger, httpClient *http.Client) *AdminUserController {
	baseURL := os.Getenv("USER_SERVICE_URL")
	if baseURL == "" {
		baseURL = "http://user-service:8085"
	}

	return &AdminUserController{
		logger:     logger,
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

func (ac *AdminUserController) ListUsers(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "ListUsers"))

	page, pageSize := parsePagination(c)
	downstreamURL := withPagination(ac.baseURL+"/users", page, pageSize)

	ac.logger.Debug("forward admin request", zap.String("handler", "ListUsers"), zap.String("method", http.MethodGet), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardGet(c.Request.Context(), ac.httpClient, downstreamURL, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "ListUsers"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "ListUsers"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminUserController) UpdateUserRole(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "UpdateUserRole"))

	userID := strings.TrimSpace(c.Param("id"))
	if !isValidPathID(userID) {
		ac.logger.Warn("invalid user id", zap.String("handler", "UpdateUserRole"), zap.String("user_id", userID))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return
	}

	bodyBytes, ok := ac.bindAndValidateBody(c, "UpdateUserRole")
	if !ok {
		return
	}

	downstreamURL := fmt.Sprintf("%s/users/%s/role", ac.baseURL, url.PathEscape(userID))
	ac.logger.Debug("forward admin request", zap.String("handler", "UpdateUserRole"), zap.String("method", http.MethodPut), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardPut(c.Request.Context(), ac.httpClient, downstreamURL, bodyBytes, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "UpdateUserRole"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "UpdateUserRole"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminUserController) DeleteUser(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "DeleteUser"))

	userID := strings.TrimSpace(c.Param("id"))
	if !isValidPathID(userID) {
		ac.logger.Warn("invalid user id", zap.String("handler", "DeleteUser"), zap.String("user_id", userID))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return
	}

	downstreamURL := fmt.Sprintf("%s/users/%s", ac.baseURL, url.PathEscape(userID))
	ac.logger.Debug("forward admin request", zap.String("handler", "DeleteUser"), zap.String("method", http.MethodDelete), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardDelete(c.Request.Context(), ac.httpClient, downstreamURL, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "DeleteUser"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "DeleteUser"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminUserController) bindAndValidateBody(c *gin.Context, handler string) ([]byte, bool) {
	var payload adminUserRoleUpdateRequest
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

func (ac *AdminUserController) handleForwardResponse(c *gin.Context, statusCode int, responseBody []byte) {
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
