package controllers

import (
	"fmt"
	"log"
	"net/http"
	"order-service/middleware"
	"order-service/services"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type OrderController struct {
	orderService *services.OrderService
}

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

	if err := oc.orderService.CreateOrder(ctx.Request.Context(), userID, &req); err != nil {
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
		log.Println("User Id is missing")
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}

	page, limit := parsePaginationParams(ctx)

	result, serviceErr := oc.orderService.GetUserOrders(ctx.Request.Context(), userID, page, limit)

	if serviceErr != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch orders"})
		fmt.Printf("Error: %v\n", serviceErr)
		return
	}

	if result == nil {
		fmt.Printf("Error: result is nil\n")
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

	result, err := oc.orderService.GetAllOrders(ctx.Request.Context(), userID, page, limit)
	if err != nil {
		// ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch orders"})
		fmt.Printf("Error: %v\n", err)

		return
	}

	ctx.JSON(http.StatusOK, result)
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
