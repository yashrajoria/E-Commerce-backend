// Package controllers provides HTTP request handlers for order placement, tracking, and cancellations.
package controllers

import (
	"context"
	"net/http"
	"order-service/middleware"
	"order-service/services"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type contextKey string

// OrderController represents order controller.
type OrderController struct {
	orderService *services.OrderService
}

// NewOrderController creates and initializes a new OrderController instance.
func NewOrderController(orderService *services.OrderService) *OrderController {
	return &OrderController{
		orderService: orderService,
	}
}

// CreateOrder handles order creation requests
func (oc *OrderController) CreateOrder(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req services.CreateOrderRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	if req.StoreID == nil {
		if storeIDStr := ctx.GetHeader("X-Store-ID"); storeIDStr != "" {
			if sid, err := uuid.Parse(storeIDStr); err == nil {
				req.StoreID = &sid
			}
		}
	}

	email := ""
	if emailVal, exists := ctx.Get("email"); exists {
		if emailStr, ok := emailVal.(string); ok {
			email = emailStr
		}
	}

	// Support idempotency: accept Idempotency-Key header and attach to context for downstream handling.
	// Validate format synchronously so malformed keys fail fast with 400
	// instead of being accepted (202) and failing async later.
	idemKey := ctx.GetHeader("Idempotency-Key")
	reqCtx := ctx.Request.Context()
	if idemKey != "" {
		if !services.ValidateIdempotencyKey(idemKey) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid Idempotency-Key format: must match ^[a-zA-Z0-9_:\\-]{1,128}$"})
			return
		}
		reqCtx = context.WithValue(reqCtx, services.IdempotencyKeyContextKey, idemKey)
	}

	if err := oc.orderService.CreateOrder(reqCtx, userID, email, &req); err != nil {
		ctx.JSON(err.StatusCode, gin.H{"error": err.Message})
		return
	}

	ctx.JSON(http.StatusAccepted, gin.H{"message": "Order creation started"})
}

// GetOrders returns paginated orders for the authenticated user
func (oc *OrderController) GetOrders(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if userID == "" {
		zap.L().Warn("missing user id in context for GetOrders")
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}

	page, limit := parsePaginationParams(ctx)

	result, serviceErr := oc.orderService.GetUserOrders(ctx.Request.Context(), userID, page, limit)

	if serviceErr != nil {
		status := serviceErr.StatusCode
		if status == 0 {
			status = http.StatusInternalServerError
		}
		zap.L().Error("failed to fetch user orders", zap.Int("status", status), zap.String("user_id", userID), zap.String("err", serviceErr.Message))
		ctx.JSON(status, gin.H{"error": serviceErr.Message})
		return
	}

	if result == nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch orders"})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// GetAllOrders returns paginated orders for all users (admin only)
func (oc *OrderController) GetAllOrders(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	role, exists := ctx.Get("role")
	if !exists {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	roleStr, ok := role.(string)
	if !ok || roleStr != "admin" {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	page, limit := parsePaginationParams(ctx)

	storeIDStr := ctx.GetHeader("X-Store-ID")
	if storeIDStr == "" {
		storeIDStr = ctx.Query("store_id")
	}

	var result *services.OrderResponse
	var serviceErr *services.ServiceError

	if storeIDStr != "" {
		if storeID, err := uuid.Parse(storeIDStr); err == nil {
			result, serviceErr = oc.orderService.GetStoreOrders(ctx.Request.Context(), storeID, page, limit)
		} else {
			result, serviceErr = oc.orderService.GetAllOrders(ctx.Request.Context(), userID, page, limit)
		}
	} else {
		result, serviceErr = oc.orderService.GetAllOrders(ctx.Request.Context(), userID, page, limit)
	}
	if serviceErr != nil {
		status := serviceErr.StatusCode
		if status == 0 {
			status = http.StatusInternalServerError
		}
		msg := serviceErr.Message
		if msg == "" {
			msg = "Failed to fetch orders"
		}
		ctx.JSON(status, gin.H{"error": msg})
		return
	}

	if result == nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch orders"})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// GetRevenueStats handles GET /orders/admin/revenue-stats, computing platform-wide sales and revenue analytics.
func (oc *OrderController) GetRevenueStats(ctx *gin.Context) {
	stats, serviceErr := oc.orderService.GetRevenueStats(ctx.Request.Context())
	if serviceErr != nil {
		ctx.JSON(serviceErr.StatusCode, gin.H{"error": serviceErr.Message})
		return
	}
	ctx.JSON(http.StatusOK, stats)
}

// GetOrderByID returns a specific order for the authenticated user
func (oc *OrderController) GetOrderByID(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	orderID := ctx.Param("id")
	orderUUID, err := uuid.Parse(orderID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID format"})
		return
	}

	order, serviceErr := oc.orderService.GetOrderByID(ctx.Request.Context(), userID, orderUUID)
	if serviceErr != nil {
		ctx.JSON(serviceErr.StatusCode, gin.H{"error": serviceErr.Message})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"order": order})
}

// CancelOrder handles admin cancellation of an order.
func (oc *OrderController) CancelOrder(ctx *gin.Context) {
	adminID, err := middleware.GetUserID(ctx)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	orderID := ctx.Param("id")
	orderUUID, err := uuid.Parse(orderID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID format"})
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	_ = ctx.ShouldBindJSON(&body) // reason is optional; ignore malformed/empty body

	order, serviceErr := oc.orderService.CancelOrder(ctx.Request.Context(), orderUUID, adminID, body.Reason)
	if serviceErr != nil {
		ctx.JSON(serviceErr.StatusCode, gin.H{"error": serviceErr.Message})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"order": order})
}

// RefundOrder handles refund requests for an order (full or partial).
func (oc *OrderController) RefundOrder(ctx *gin.Context) {
	callerID, err := middleware.GetUserID(ctx)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	orderID := ctx.Param("id")
	orderUUID, err := uuid.Parse(orderID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID format"})
		return
	}

	var req services.RefundOrderRequest
	if ctx.Request.Body != nil && ctx.Request.ContentLength > 0 {
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
			return
		}
	}

	callerRole := ""
	if roleVal, exists := ctx.Get("role"); exists {
		if roleStr, ok := roleVal.(string); ok {
			callerRole = roleStr
		}
	}

	idemKey := ctx.GetHeader("Idempotency-Key")
	reqCtx := ctx.Request.Context()
	if idemKey != "" {
		if !services.ValidateIdempotencyKey(idemKey) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid Idempotency-Key format: must match ^[a-zA-Z0-9_:\\-]{1,128}$"})
			return
		}
		reqCtx = context.WithValue(reqCtx, services.IdempotencyKeyContextKey, idemKey)
	}

	order, serviceErr := oc.orderService.RefundOrder(reqCtx, orderUUID, callerID, callerRole, &req)
	if serviceErr != nil {
		ctx.JSON(serviceErr.StatusCode, gin.H{"error": serviceErr.Message})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"order": order, "message": "Order refunded successfully"})
}

// parsePaginationParams extracts and validates pagination parameters
func parsePaginationParams(ctx *gin.Context) (int, int) {
	const MaxLimit = 100
	const DefaultPage = 1
	const DefaultLimit = 10

	page := ctx.DefaultQuery("page", "1")
	limit := ctx.DefaultQuery("limit", "10")

	pageInt := DefaultPage
	limitInt := DefaultLimit

	if p, err := strconv.Atoi(page); err == nil && p > 0 {
		pageInt = p
	}

	if l, err := strconv.Atoi(limit); err == nil && l > 0 {
		limitInt = l
		if limitInt > MaxLimit {
			limitInt = MaxLimit
		}
	}

	return pageInt, limitInt
}
