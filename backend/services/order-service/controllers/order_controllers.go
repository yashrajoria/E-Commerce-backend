package controllers

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/yashrajoria/order-service/database"
	"github.com/yashrajoria/order-service/models"
	"gorm.io/gorm"
)

// Payload struct to bind JSON with order + items
type CreateOrderRequest struct {
	UserID uuid.UUID        `json:"user_id" binding:"required"`
	Items  []OrderItemInput `json:"items" binding:"required,dive,required"`
	Amount int              `json:"amount" binding:"required"`
	Status string           `json:"status"`
}

// Define input for order items (no ID needed here)
type OrderItemInput struct {
	ProductID uuid.UUID `json:"product_id" binding:"required"`
	Quantity  int       `json:"quantity" binding:"required,min=1"`
}

func CreateOrder(c *gin.Context) {
	var req CreateOrderRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Println("Invalid data:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	order := models.Order{
		UserID: req.UserID,
		Amount: req.Amount,
		Status: req.Status,
	}

	// Use transaction for atomic operation
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		// Save order
		if err := tx.Create(&order).Error; err != nil {
			return err
		}

		// Prepare and insert order items
		var orderItems []models.OrderItem
		for _, item := range req.Items {
			orderItems = append(orderItems, models.OrderItem{
				ID:        uuid.New(),
				OrderID:   order.ID,
				ProductID: item.ProductID,
				Quantity:  item.Quantity,
			})
		}

		if err := tx.Create(&orderItems).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		log.Println("Failed to create order:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create order"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "Order created successfully", "order_id": order.ID})

}

func GetOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	offset := (page - 1) * limit
	log.Println("Page:", page, "Limit:", limit, "Offset:", offset)

	var orders []models.Order
	var totalOrders int64
	database.DB.Model(&models.Order{}).Count(&totalOrders)

	if err := database.DB.Preload("OrderItems").Offset(offset).Limit(limit).Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch orders"})
		return
	}

	if len(orders) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "No orders found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"orders":       orders,
		"page":         page,
		"limit":        limit,
		"total_orders": totalOrders,
		"total_pages":  (totalOrders + int64(limit) - 1) / int64(limit),
		"has_more":     totalOrders > int64(page*limit),
		"message":      "Orders fetched successfully",
	})
}
