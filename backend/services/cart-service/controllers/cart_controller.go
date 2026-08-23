package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"cart-service/config"
	"cart-service/database"
	"cart-service/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"github.com/yashrajoria/common/internalauth"
	"go.uber.org/zap"
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

	ctx := c.Request.Context()

	cart, err := cc.Repo.GetCart(ctx, userID)
	if err != nil {
		zap.L().Error("{GET CART FAILED} for user", zap.String("userID", userID), zap.Error(err))
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
		Quantity  int    `json:"quantity" binding:"required,min=1,max=999"`
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

	ctx := c.Request.Context()

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

	ctx := c.Request.Context()

	cart, err := cc.Repo.GetCart(ctx, userID)
	if err != nil {
		zap.L().Error("[RemoveItem] Failed to get cart", zap.String("userID", userID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get cart"})
		return
	}
	if cart == nil {
		zap.L().Warn("[RemoveItem] Cart not found", zap.String("userID", userID))

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
		zap.L().Error("[RemoveItem] Failed to update cart", zap.String("userID", userID), zap.Error(err))

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

	ctx := c.Request.Context()

	err := cc.Repo.DeleteCart(ctx, userID)
	if err != nil {
		zap.L().Error("[ClearCart] Failed to clear cart", zap.String("userID", userID), zap.Error(err))

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to clear cart"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "cart cleared"})
}

// ApplyCoupon and adds a coupon code to the user's cart
func (cc *CartController) ApplyCoupon(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		if v, err := c.Cookie("user_id"); err == nil && v != "" {
			userID = v
		}
	}

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		CouponCode string `json:"coupon_code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	ctx := c.Request.Context()
	cart, err := cc.Repo.GetCart(ctx, userID)
	if err != nil {
		zap.L().Error("failed to fetch cart", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	if cart == nil {
		cart = &models.Cart{
			UserID: userID,
			Items:  []models.CartItem{},
		}
	}

	cart.CouponCode = req.CouponCode
	if err := cc.Repo.SaveCart(ctx, cart); err != nil {
		zap.L().Error("failed to update cart with coupon", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to apply coupon"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "coupon applied successfully",
		"coupon_code": req.CouponCode,
	})
}

// Checkout initiates the order process via SNS and clears it
func (cc *CartController) Checkout(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		if v, err := c.Cookie("user_id"); err == nil && v != "" {
			userID = v
		}
	}
	if userID == "" {
		zap.L().Warn("[Checkout] Unauthorized: missing or empty user ID header/cookie")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	ctx := c.Request.Context()

	cart, err := cc.Repo.GetCart(ctx, userID)
	if err != nil || cart == nil {
		zap.L().Error("[Checkout] Cart not found or error", zap.String("userID", userID), zap.Error(err))

		c.JSON(http.StatusNotFound, gin.H{"error": "cart not found"})
		return
	}
	// support idempotency: if Idempotency-Key header present, check Redis for existing order.
	// Scope by user so two different users never collide on the same header value.
	rawIdemKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	scopedIdemKey := ""
	if rawIdemKey != "" {
		scopedIdemKey = userID + ":" + rawIdemKey
		if existing, err := cc.Repo.GetIdempotency(ctx, scopedIdemKey); err == nil && existing != "" {
			zap.L().Info("[Checkout] Returning cached order_id (same request retried)", zap.String("order_id", existing), zap.String("scoped_idempotency_key", scopedIdemKey))
			c.JSON(http.StatusOK, gin.H{"order_id": existing, "status": "PENDING"})
			return
		}
	}

	// Validate products exist via a single product-service batch-validate call before publishing
	productServiceURL := os.Getenv("PRODUCT_SERVICE_URL")
	if productServiceURL == "" {
		productServiceURL = "http://product-service:8082"
	}

	productIDs := make([]string, len(cart.Items))
	for i, it := range cart.Items {
		productIDs[i] = it.ProductID
	}

	invalid, err := cc.validateProductsBatch(ctx, c, productServiceURL, productIDs)
	if err != nil {
		zap.L().Error("[Checkout] Failed to batch-validate cart products", zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to validate cart items"})
		return
	}

	if len(invalid) > 0 {
		// return a clear frontend-visible error listing invalid items
		c.JSON(http.StatusBadRequest, gin.H{
			"error":               "some items in cart are invalid or missing",
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
		IdempotencyKey: scopedIdemKey,
		Timestamp:      time.Now(),
		OrderID:        orderID,
		CouponCode:     cart.CouponCode,
	}

	eventBytes, err := json.Marshal(event)
	if err != nil {
		zap.L().Error("failed to marshal event", zap.Error(err))
		return
	}
	topicArn := os.Getenv("ORDER_SNS_TOPIC_ARN")
	if topicArn == "" {
		topicArn = "arn:aws:sns:eu-west-2:000000000000:order-events"
	}

	// Log topic and payload size for debugging
	// zap.L().Debug("[CHECKOUT] publishing SNS", zap.String("topicArn", topicArn), zap.Int("payload_len", len(eventBytes)), zap.String("userID", userID))

	if err := cc.SNSClient.Publish(ctx, topicArn, eventBytes); err != nil {
		zap.L().Error("[Checkout] Failed to send SNS event", zap.String("userID", userID), zap.String("topic", topicArn), zap.Error(err))

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to publish checkout event"})
		return
	}

	// Only persist idempotency mapping AFTER SNS publish succeeds
	// This prevents caching failed orders
	if scopedIdemKey != "" {
		if err := cc.Repo.SetIdempotency(ctx, scopedIdemKey, orderID, cc.Config.CartTTL); err != nil {
			zap.L().Warn("[Checkout] Failed to persist idempotency key", zap.String("orderID", orderID), zap.Error(err))
			// Non-fatal: order was created and published, so continue
		}
	}

	c.JSON(http.StatusOK, gin.H{"order_id": orderID, "status": "PENDING"})
}

// validateProductsBatch validates all given product IDs in a single call to
// product-service's internal batch-validate endpoint, returning the IDs that
// are missing/invalid. Replaces a per-item HTTP round trip.
func (cc *CartController) validateProductsBatch(ctx context.Context, c *gin.Context, productServiceURL string, productIDs []string) ([]string, error) {
	body, err := json.Marshal(map[string][]string{"product_ids": productIDs})
	if err != nil {
		return nil, err
	}

	reqUrl := productServiceURL + "/products/internal/batch-validate"
	req, err := http.NewRequestWithContext(ctx, "POST", reqUrl, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if corID := c.GetHeader("X-Correlation-ID"); corID != "" {
		req.Header.Set("X-Correlation-ID", corID)
	}
	internalauth.Apply(req)

	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("batch-validate returned status %d", resp.StatusCode)
	}

	var result struct {
		InvalidProductIDs []string `json:"invalid_product_ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.InvalidProductIDs, nil
}
