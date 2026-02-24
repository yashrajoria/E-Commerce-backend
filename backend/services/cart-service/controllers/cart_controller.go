package controllers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"cart-service/config"
	"cart-service/database"
	"cart-service/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
)

type CartController struct {
	Repo      *database.CartRepository
	SNSClient *aws_pkg.SNSClient
	Config    config.Config
}

func NewCartController(repo *database.CartRepository, snsClient *aws_pkg.SNSClient, cfg config.Config) *CartController {
	return &CartController{
		Repo:      repo,
		SNSClient: snsClient,
		Config:    cfg,
	}
}

// GetCart returns the current cart for a user
func (cc *CartController) GetCart(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		if v, err := c.Cookie("user_id"); err == nil && v != "" {
			userID = v
		}
	}
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
		if v, err := c.Cookie("user_id"); err == nil && v != "" {
			userID = v
		}
	}
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
		if v, err := c.Cookie("user_id"); err == nil && v != "" {
			userID = v
		}
	}
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
		if v, err := c.Cookie("user_id"); err == nil && v != "" {
			userID = v
		}
	}
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

// Checkout publishes the cart to SNS and clears it
func (cc *CartController) Checkout(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		if v, err := c.Cookie("user_id"); err == nil && v != "" {
			userID = v
		}
	}
	if userID == "" {
		log.Println("❌ [Checkout] Unauthorized: missing or empty user ID header/cookie")
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
	// support idempotency: if Idempotency-Key header present, check Redis for existing order
	idemKey := c.GetHeader("Idempotency-Key")
	if idemKey != "" {
		if existing, err := cc.Repo.GetIdempotency(context.Background(), idemKey); err == nil && existing != "" {
			c.JSON(http.StatusOK, gin.H{"order_id": existing, "status": "PENDING"})
			return
		}
	}

	// Validate products exist via product-service internal API before publishing
	invalid := []string{}
	productServiceURL := os.Getenv("PRODUCT_SERVICE_URL")
	if productServiceURL == "" {
		productServiceURL = "http://product-service:8082"
	}
	// simple per-item check
	for _, it := range cart.Items {
		// GET /products/internal/:id
		resp, err := http.Get(productServiceURL + "/products/internal/" + it.ProductID)
		if err != nil || resp.StatusCode != http.StatusOK {
			invalid = append(invalid, it.ProductID)
			continue
		}
		// drain body
		_ = resp.Body.Close()
	}

	if len(invalid) > 0 {
		// return a clear frontend-visible error listing invalid items
		c.JSON(http.StatusBadRequest, gin.H{
			"error":            "some items in cart are invalid or missing",
			"invalid_product_ids": invalid,
		})
		return
	}

	orderID := uuid.New().String()
	// Build SNS payload
	event := models.CheckoutEvent{
		Event:          "checkout.requested",
		UserID:         userID,
		Items:          cart.Items,
		IdempotencyKey: idemKey,
		Timestamp:      time.Now(),
		OrderID:        orderID,
	}

	eventBytes, _ := json.Marshal(event)
	topicArn := os.Getenv("ORDER_SNS_TOPIC_ARN")
	if topicArn == "" {
		topicArn = "arn:aws:sns:eu-west-2:000000000000:order-events"
	}

	// Log topic and payload size for debugging
	// log.Printf("[CHECKOUT] publishing SNS topicArn=%q payload_len=%d userID=%s", topicArn, len(eventBytes), userID)

	if err := cc.SNSClient.Publish(ctx, topicArn, eventBytes); err != nil {
		log.Printf("❌ [Checkout] Failed to send SNS event for userID=%s topic=%s: %v", userID, topicArn, err)

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to publish checkout event"})
		return
	}

	// persist idempotency mapping if key provided
	if idemKey != "" {
		_ = cc.Repo.SetIdempotency(context.Background(), idemKey, orderID, cc.Config.CartTTL)
	}

	// Clear cart after sending
	// _ = cc.Repo.DeleteCart(ctx, userID)

	c.JSON(http.StatusOK, gin.H{"order_id": orderID, "status": "PENDING"})
}
