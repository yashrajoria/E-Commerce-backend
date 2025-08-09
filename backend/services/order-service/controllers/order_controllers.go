package controllers

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"order-service/database"
	"order-service/middleware"
	"order-service/models"
	"order-service/services"
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

func CreateOrder(c *gin.Context) {
	userIDStr, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userID, _ := uuid.Parse(userIDStr)

	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	// Generate order number
	orderNumber := fmt.Sprintf("ORD-%d-%s", time.Now().Year(), uuid.New().String()[:6])

	var totalAmount int
	var orderItems []models.OrderItem
	productServiceURL := c.GetString("product_service_url")

	// Fetch and validate products
	for _, item := range req.Items {
		product, err := services.FetchProductByID(c.Request.Context(), productServiceURL, item.ProductID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Product fetch error: %v", err)})
			return
		}
		if product.Stock < item.Quantity {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Product out of stock: %s", item.ProductID)})
			return
		}

		price := product.Price // already a float64!
		totalAmount += int(math.Round(price*100)) * item.Quantity
		orderItems = append(orderItems, models.OrderItem{
			ID:        uuid.New(),
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			Price:     int(math.Round(price * 100)), // store in minor units
		})
	}

	// Create transaction
	order := models.Order{
		UserID:      userID,
		Amount:      totalAmount,
		Status:      "pending",
		OrderNumber: orderNumber,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := database.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		for i := range orderItems {
			orderItems[i].OrderID = order.ID
		}
		return tx.Create(&orderItems).Error
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Resource not found"})
		} else {

			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create order"})
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Order created", "order_id": order.ID})
}

// MaxLimit caps the number of orders per page to prevent abuse
const MaxLimit = 100

func GetOrders(c *gin.Context) {
	userIDStr, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userID, _ := uuid.Parse(userIDStr)

	// Parse and sanitize pagination parameters
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
	offset := (page - 1) * limit

	var orders []models.Order
	var totalOrders int64

	// Count total for this user
	if err := database.DB.WithContext(c.Request.Context()).
		Model(&models.Order{}).
		Where("user_id = ?", userID).
		Count(&totalOrders).Error; err != nil {

		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count orders"})
		return
	}

	// Fetch paginated orders with preloaded items
	if err := database.DB.WithContext(c.Request.Context()).
		Preload("OrderItems").
		Where("user_id = ?", userID).
		Offset(offset).
		Limit(limit).
		Order("created_at DESC").
		Find(&orders).Error; err != nil {
		// TODO: Add structured logging here.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch orders"})
		return
	}

	if len(orders) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"orders": []models.Order{},
			"meta": gin.H{
				"page":         page,
				"limit":        limit,
				"total_orders": totalOrders,
				"total_pages":  0,
				"has_more":     false,
			},
		})
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

func GetOrderByID(c *gin.Context) {
	userIDStr, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID format"})
		return
	}
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
