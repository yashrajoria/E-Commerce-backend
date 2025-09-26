package controllers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"order-service/database"
	"order-service/kafka"
	"order-service/middleware"
	"order-service/models"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"gorm.io/gorm"
)

type CreateOrderRequest struct {
	Items []struct {
		ProductID uuid.UUID `json:"product_id" binding:"required"`
		Quantity  int       `json:"quantity" binding:"required,min=1"`
	} `json:"items" binding:"required,dive"`
}

type OrderController struct {
	KafkaProducer *kafka.Producer // Add your Kafka producer here
}

func (c *OrderController) CreateOrder(ctx *gin.Context) {
	userIDStr, err := middleware.GetUserID(ctx)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req CreateOrderRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	// Validate products & quantities same way you do currently (fetch product & check stock)

	// Build event items in the required event format:
	eventItems := make([]models.CheckoutItem, 0, len(req.Items))
	for _, item := range req.Items {
		eventItems = append(eventItems, models.CheckoutItem{
			ProductID: item.ProductID.String(),
			Quantity:  item.Quantity,
		})
	}

	// Create the checkout event
	checkoutEvent := models.CheckoutEvent{
		UserID:    userIDStr,
		Items:     eventItems,
		Timestamp: time.Now(),
	}

	eventBytes, err := json.Marshal(checkoutEvent)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encode checkout event"})
		return
	}

	// Publish the event to Kafka topic "checkout.requested"
	if err := c.KafkaProducer.Publish("checkout.requested", eventBytes); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to publish checkout event"})
		return
	}

	ctx.JSON(http.StatusAccepted, gin.H{"message": "Order creation started"})
}

// MaxLimit caps the number of orders per page to prevent abuse
const MaxLimit = 100

// For regular users: GetOrders returns paginated orders only for the authenticated user
func GetOrders(c *gin.Context) {
	// userIDStr, err := middleware.GetUserID(c)
	userID := c.GetHeader("X-User-ID")

	log.Println("[GetOrders] User ID:", userID)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	// userID, _ := uuid.Parse(userIDStr)

	page, limit := parsePaginationParams(c)

	var orders []models.Order
	var totalOrders int64

	query := database.DB.WithContext(c.Request.Context()).
		Model(&models.Order{}).
		Where("user_id = ?", userID)

	if err := query.Count(&totalOrders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count orders"})
		return
	}

	if err := query.Preload("OrderItems").
		Offset((page - 1) * limit).
		Limit(limit).
		Order("created_at DESC").
		Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch orders"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"orders": orders,
		"meta": gin.H{
			"page":         page,
			"limit":        limit,
			"total_orders": totalOrders,
			"total_pages":  (totalOrders + int64(limit) - 1) / int64(limit),
			"has_more":     totalOrders > int64(page*limit),
		},
	})
}

// Admin-only: GetAllOrders returns paginated orders for all users
func GetAllOrders(c *gin.Context) {
	role, _ := c.Get("role")
	log.Println("[GetAllOrders] User Role:", role)
	page, limit := parsePaginationParams(c)

	var orders []models.Order
	var totalOrders int64

	query := database.DB.WithContext(c.Request.Context()).Model(&models.Order{})

	if err := query.Count(&totalOrders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count orders"})
		return
	}

	if err := query.Preload("OrderItems").
		Offset((page - 1) * limit).
		Limit(limit).
		Order("created_at DESC").
		Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch orders"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"orders": orders,
		"meta": gin.H{
			"page":         page,
			"limit":        limit,
			"total_orders": totalOrders,
			"total_pages":  (totalOrders + int64(limit) - 1) / int64(limit),
			"has_more":     totalOrders > int64(page*limit),
		},
	})
}

// Helper function to parse and sanitize pagination parameters
func parsePaginationParams(c *gin.Context) (int, int) {
	const MaxLimit = 100
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	return page, limit
}

func GetOrderByID(c *gin.Context) {
	// userIDStr, err := middleware.GetUserID(c)
	userID := c.GetHeader("X-User-ID")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	// userID, err := uuid.Parse(userIDStr)
	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID format"})
	// 	return
	// }
	orderID := c.Param("id")

	// Validate UUID format for orderID
	orderUUID, err := uuid.Parse(orderID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID format"})
		return
	}

	var order models.Order
	if err := database.DB.WithContext(c.Request.Context()).
		Preload("OrderItems").
		Where("id = ? AND user_id = ?", orderUUID, userID).
		First(&order).Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
		} else {
			// TODO: Add structured logging here.
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch order"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"order": order,
	})
}
