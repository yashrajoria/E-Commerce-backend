package controllers

import (
	"context"
	"log"
	"net/http"
	"time"

	"cart-service/config"
	"cart-service/database"
	"cart-service/kafka"
	"cart-service/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type CartController struct {
	Repo     *database.CartRepository
	Producer *kafka.Producer
	Config   config.Config
}

func NewCartController(repo *database.CartRepository, producer *kafka.Producer, cfg config.Config) *CartController {
	return &CartController{
		Repo:     repo,
		Producer: producer,
		Config:   cfg,
	}
}

// GetCart returns the current cart for a user
func (cc *CartController) GetCart(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authorized"})
		return
	}

	ctx := context.Background()

	cart, err := cc.Repo.GetCart(ctx, userID)
	if err != nil {
		log.Printf("{GET CART FAILED} for user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get cart"})
		return
	}

	if cart == nil {
		cart = &models.Cart{
			UserID: userID,
			Items:  []models.CartItem{},
		}
	}

	c.JSON(http.StatusOK, cart)
}

// AddItem adds or updates an item in the cart
type AddItemsRequest struct {
	Items []struct {
		ProductID string `json:"product_id" binding:"required,uuid"`
		Quantity  int    `json:"quantity" binding:"required,min=1"`
	} `json:"items" binding:"required,dive"`
}

func (cc *CartController) AddItems(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authorized"})
		return
	}

	var req AddItemsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	ctx := context.Background()

	cart, err := cc.Repo.GetCart(ctx, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get cart"})
		return
	}

	if cart == nil {
		cart = &models.Cart{
			UserID: userID,
			Items:  []models.CartItem{},
		}
	}

	// Update cart items: increment quantities if product exists, else add new
	for _, newItem := range req.Items {
		found := false
		for i, existing := range cart.Items {
			if existing.ProductID == newItem.ProductID {
				cart.Items[i].Quantity += newItem.Quantity
				found = true
				break
			}
		}
		if !found {
			cart.Items = append(cart.Items, models.CartItem{
				ProductID: newItem.ProductID,
				Quantity:  newItem.Quantity,
			})
		}
	}

	if err := cc.Repo.SaveCart(ctx, cart); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save cart"})
		return
	}

	c.JSON(http.StatusOK, cart)
}

// RemoveItem removes a specific item from the cart
func (cc *CartController) RemoveItem(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	productID := c.Param("product_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authorized"})
		return
	}

	ctx := context.Background()

	cart, _ := cc.Repo.GetCart(ctx, userID)
	if cart == nil {
		log.Printf("⚠️ [RemoveItem] Cart not found for userID=%s", userID)

		c.JSON(http.StatusNotFound, gin.H{"error": "cart not found"})
		return
	}

	newItems := []models.CartItem{}
	for _, item := range cart.Items {
		if item.ProductID != productID {
			newItems = append(newItems, item)
		}
	}
	cart.Items = newItems

	if err := cc.Repo.SaveCart(ctx, cart); err != nil {
		log.Printf("❌ [RemoveItem] Failed to update cart for userID=%s: %v", userID, err)

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update cart"})
		return
	}

	c.JSON(http.StatusOK, cart)
}

// ClearCart removes all items from the cart
func (cc *CartController) ClearCart(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authorized"})
		return
	}

	ctx := context.Background()

	err := cc.Repo.DeleteCart(ctx, userID)
	if err != nil {
		log.Printf("❌ [ClearCart] Failed to clear cart for userID=%s: %v", userID, err)

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to clear cart"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "cart cleared"})
}

// Checkout publishes the cart to Kafka and clears it
func (cc *CartController) Checkout(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authorized"})
		return
	}

	if userID == "" {
		log.Println("❌ [Checkout] Unauthorized: missing or empty user ID header")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	ctx := context.Background()

	cart, err := cc.Repo.GetCart(ctx, userID)
	if err != nil || cart == nil {
		log.Printf("❌ [Checkout] Cart not found or error for userID=%s: %v", userID, err)

		c.JSON(http.StatusNotFound, gin.H{"error": "cart not found"})
		return
	}
	orderID := uuid.New().String()
	// Build Kafka payload
	event := models.CheckoutEvent{
		Event:     "checkout.requested",
		UserID:    userID,
		Items:     cart.Items,
		Timestamp: time.Now(),
		OrderID:   orderID,
	}

	if err := cc.Producer.SendCheckoutEvent(event); err != nil {
		log.Printf("❌ [Checkout] Failed to send Kafka event for userID=%s: %v", userID, err)

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to publish checkout event"})
		return
	}

	// Clear cart after sending
	// _ = cc.Repo.DeleteCart(ctx, userID)

	c.JSON(http.StatusOK, gin.H{"order_id": orderID, "status": "PENDING"})
}
