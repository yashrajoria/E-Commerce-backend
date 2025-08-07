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
func (cc *CartController) AddItem(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	var item models.CartItem

	if err := c.ShouldBindJSON(&item); err != nil {
		log.Printf("[AddItem] Invalid payload: %v", err)

		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	ctx := context.Background()
	cart, _ := cc.Repo.GetCart(ctx, userID)
	if cart == nil {
		cart = &models.Cart{
			UserID: userID,
			Items:  []models.CartItem{},
		}
	}

	found := false
	for i, existing := range cart.Items {
		if existing.ProductID == item.ProductID {
			cart.Items[i].Quantity += item.Quantity
			found = true
			break
		}
	}
	if !found {
		cart.Items = append(cart.Items, item)
	}

	if err := cc.Repo.SaveCart(ctx, cart); err != nil {
		log.Printf("❌ [AddItem] Failed to save cart: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save cart"})
		return
	}

	c.JSON(http.StatusOK, cart)
}

// RemoveItem removes a specific item from the cart
func (cc *CartController) RemoveItem(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	productID := c.Param("product_id")
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

	// Build Kafka payload
	event := models.CheckoutEvent{
		Event:     "checkout.requested",
		UserID:    userID,
		Items:     cart.Items,
		Timestamp: time.Now(),
	}

	if err := cc.Producer.SendCheckoutEvent(event); err != nil {
		log.Printf("❌ [Checkout] Failed to send Kafka event for userID=%s: %v", userID, err)

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to publish checkout event"})
		return
	}

	// Clear cart after sending
	_ = cc.Repo.DeleteCart(ctx, userID)

	c.JSON(http.StatusOK, gin.H{"message": "checkout initiated"})
}
