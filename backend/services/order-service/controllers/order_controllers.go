package controllers

import (
	"log"
	"net/http"

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
