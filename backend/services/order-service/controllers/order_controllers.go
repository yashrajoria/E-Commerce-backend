package controllers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	nanoid "github.com/matoous/go-nanoid/v2"

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
	Price     float64   `json:"price"`
}

type Product struct {
	ID    uuid.UUID `json:"id"`
	Price int       `json:"price"`
	Stock int       `json:"stock"`
}

func GenerateOrderNumber() (string, error) {
	year := time.Now().Year()
	id, err := nanoid.Generate("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", 6)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ORD-%d-%s", year, id), nil
}

func FetchProductByID(productID uuid.UUID) (*Product, error) {
	productServiceURL := fmt.Sprintf("http://product-service:8082/products/internal/%s", productID.String())

	resp, err := http.Get(productServiceURL)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("product service returned %d", resp.StatusCode)
	}

	var product Product
	if err := json.NewDecoder(resp.Body).Decode(&product); err != nil {
		return nil, err
	}

	return &product, nil
}

func CreateOrder(c *gin.Context) {
	var req CreateOrderRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Println("Invalid data:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	orderNumber, err := GenerateOrderNumber()
	if err != nil {
		log.Println("Failed to generate order number:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate order number"})
		return
	}

	order := models.Order{
		UserID:      req.UserID,
		Amount:      req.Amount,
		Status:      req.Status,
		OrderNumber: orderNumber,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Use transaction for atomic operation
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		// First create the order to get the generated ID
		if err := tx.Create(&order).Error; err != nil {
			return err
		}

		// Now that order.ID is populated, proceed with items
		var orderItems []models.OrderItem
		for _, item := range req.Items {
			product, err := FetchProductByID(item.ProductID)
			if err != nil {
				return fmt.Errorf("product fetch error: %w", err)
			}

			if product.Stock < item.Quantity {
				return fmt.Errorf("product out of stock: %s", item.ProductID.String())
			}

			orderItems = append(orderItems, models.OrderItem{
				ID:        uuid.New(),
				OrderID:   order.ID, // ✅ Now order.ID is valid
				ProductID: item.ProductID,
				Quantity:  item.Quantity,
				Price:     int(item.Price),
			})
		}

		if err := tx.Create(&orderItems).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		log.Println("❌ Failed to create order:", err)

		// Handle known errors
		if err.Error() == "product fetch error" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid product"})
		} else if strings.Contains(err.Error(), "out of stock") {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create order"})
		}
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
		"orders": orders,
		"meta": gin.H{
			"page":         page,
			"limit":        limit,
			"total_orders": totalOrders,
			"total_pages":  (totalOrders + int64(limit) - 1) / int64(limit),
			"has_more":     totalOrders > int64(page*limit),
		},
		"message": "Orders fetched successfully",
	})
}

func GetOrderByID(c *gin.Context) {
	orderID := c.Param("id")
	var order models.Order

	if err := database.DB.Preload("OrderItems").Where("id = ?", orderID).First(&order).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch order"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"order":   order,
		"message": "Order fetched successfully",
	})
}
