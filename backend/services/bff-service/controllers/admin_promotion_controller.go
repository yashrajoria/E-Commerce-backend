package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"bff-service/utils"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AdminPromotionController struct {
	logger     *zap.Logger
	httpClient *http.Client
	baseURL    string
}

type adminCouponCreateRequest struct {
	Code          string    `json:"code" binding:"required,min=3,max=64"`
	Type          string    `json:"type" binding:"required,oneof=percentage flat freeshipping"`
	Value         float64   `json:"value" binding:"required,gte=0"`
	MinOrderValue float64   `json:"min_order_value" binding:"gte=0"`
	UsageLimit    int       `json:"usage_limit" binding:"gte=0"`
	ExpiresAt     time.Time `json:"expires_at" binding:"required"`
}

type adminCouponUpdateRequest struct {
	Code          *string    `json:"code,omitempty" binding:"omitempty,min=3,max=64"`
	Type          *string    `json:"type,omitempty" binding:"omitempty,oneof=percentage flat freeshipping"`
	Value         *float64   `json:"value,omitempty" binding:"omitempty,gte=0"`
	MinOrderValue *float64   `json:"min_order_value,omitempty" binding:"omitempty,gte=0"`
	UsageLimit    *int       `json:"usage_limit,omitempty" binding:"omitempty,gte=0"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

func NewAdminPromotionController(logger *zap.Logger, httpClient *http.Client) *AdminPromotionController {
	baseURL := os.Getenv("PROMOTION_SERVICE_URL")
	if baseURL == "" {
		baseURL = "http://promotion-service:8090"
	}

	return &AdminPromotionController{
		logger:     logger,
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

func (ac *AdminPromotionController) ListCoupons(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "ListCoupons"))

	page, pageSize := parsePagination(c)
	downstreamURL := withPagination(ac.baseURL+"/coupons", page, pageSize)

	ac.logger.Debug("forward admin request", zap.String("handler", "ListCoupons"), zap.String("method", http.MethodGet), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardGet(c.Request.Context(), ac.httpClient, downstreamURL, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "ListCoupons"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "ListCoupons"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminPromotionController) CreateCoupon(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "CreateCoupon"))

	bodyBytes, ok := ac.bindCreateCouponBody(c)
	if !ok {
		return
	}

	downstreamURL := ac.baseURL + "/coupons"
	ac.logger.Debug("forward admin request", zap.String("handler", "CreateCoupon"), zap.String("method", http.MethodPost), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardPost(c.Request.Context(), ac.httpClient, downstreamURL, bodyBytes, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "CreateCoupon"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "CreateCoupon"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminPromotionController) UpdateCoupon(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "UpdateCoupon"))

	couponID := strings.TrimSpace(c.Param("id"))
	if !isValidPathID(couponID) {
		ac.logger.Warn("invalid coupon id", zap.String("handler", "UpdateCoupon"), zap.String("coupon_id", couponID))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return
	}

	bodyBytes, ok := ac.bindUpdateCouponBody(c)
	if !ok {
		return
	}

	downstreamURL := fmt.Sprintf("%s/coupons/%s", ac.baseURL, url.PathEscape(couponID))
	ac.logger.Debug("forward admin request", zap.String("handler", "UpdateCoupon"), zap.String("method", http.MethodPut), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardPut(c.Request.Context(), ac.httpClient, downstreamURL, bodyBytes, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "UpdateCoupon"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "UpdateCoupon"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminPromotionController) DeleteCoupon(c *gin.Context) {
	defer ac.logger.Debug("admin request complete", zap.String("handler", "DeleteCoupon"))

	couponID := strings.TrimSpace(c.Param("id"))
	if !isValidPathID(couponID) {
		ac.logger.Warn("invalid coupon id", zap.String("handler", "DeleteCoupon"), zap.String("coupon_id", couponID))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return
	}

	downstreamURL := fmt.Sprintf("%s/coupons/%s", ac.baseURL, url.PathEscape(couponID))
	ac.logger.Debug("forward admin request", zap.String("handler", "DeleteCoupon"), zap.String("method", http.MethodDelete), zap.String("url", downstreamURL))
	body, statusCode, err := utils.ForwardDelete(c.Request.Context(), ac.httpClient, downstreamURL, c.Request.Header)
	if err != nil {
		ac.logger.Error("downstream request failed", zap.String("handler", "DeleteCoupon"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "downstream service unavailable")
		return
	}
	ac.logger.Debug("downstream response received", zap.String("handler", "DeleteCoupon"), zap.Int("status", statusCode))

	ac.handleForwardResponse(c, statusCode, body)
}

func (ac *AdminPromotionController) bindCreateCouponBody(c *gin.Context) ([]byte, bool) {
	var payload adminCouponCreateRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		ac.logger.Warn("invalid request body", zap.String("handler", "CreateCoupon"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return nil, false
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		ac.logger.Error("failed to encode request body", zap.String("handler", "CreateCoupon"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusInternalServerError, "unexpected error")
		return nil, false
	}

	return bodyBytes, true
}

func (ac *AdminPromotionController) bindUpdateCouponBody(c *gin.Context) ([]byte, bool) {
	var payload adminCouponUpdateRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		ac.logger.Warn("invalid request body", zap.String("handler", "UpdateCoupon"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return nil, false
	}

	if payload.Code == nil && payload.Type == nil && payload.Value == nil &&
		payload.MinOrderValue == nil && payload.UsageLimit == nil && payload.ExpiresAt == nil {
		ac.logger.Warn("empty request body", zap.String("handler", "UpdateCoupon"))
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request")
		return nil, false
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		ac.logger.Error("failed to encode request body", zap.String("handler", "UpdateCoupon"), zap.Error(err))
		utils.ErrorResponse(c, http.StatusInternalServerError, "unexpected error")
		return nil, false
	}

	return bodyBytes, true
}

func (ac *AdminPromotionController) handleForwardResponse(c *gin.Context, statusCode int, responseBody []byte) {
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
