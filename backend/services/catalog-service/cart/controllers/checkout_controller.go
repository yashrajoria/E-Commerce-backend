package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"catalog-service/cart/models"
	productservices "catalog-service/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ProductValidator is the subset of the product service that cart checkout
// needs. In-process implementation: *productservices.ProductServiceDDB —
// the old HTTP batch-validate hop is gone.
type ProductValidator interface {
	GetProductsInternal(ctx context.Context, ids []uuid.UUID) ([]*productservices.ProductInternalDTO, error)
}

// errValidatorUnavailable is returned when cart checkout runs without a
// product validator wired in (fail-closed: checkout cannot proceed unvalidated).
var errValidatorUnavailable = errors.New("product validator not configured")

// Checkout handles POST /cart/checkout, validating inventory holds and emitting checkout events to initiate order placement.
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
	if len(cart.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cart is empty"})
		return
	}
	// support idempotency: if Idempotency-Key header present, check Redis for existing order.
	// Scope by user so two different users never collide on the same header value.
	// Hash the raw key so an arbitrarily long/odd client-supplied header can't
	// produce an unbounded or malformed Redis key.
	rawIdemKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	scopedIdemKey := ""
	if rawIdemKey != "" {
		hash := sha256.Sum256([]byte(rawIdemKey))
		scopedIdemKey = userID + ":" + hex.EncodeToString(hash[:])
		if existing, err := cc.Repo.GetIdempotency(ctx, scopedIdemKey); err == nil && existing != "" {
			zap.L().Info("[Checkout] Returning cached order_id (same request retried)", zap.String("order_id", existing), zap.String("scoped_idempotency_key", scopedIdemKey))
			c.JSON(http.StatusOK, gin.H{"order_id": existing, "status": "PENDING"})
			return
		}
	}

	// Validate products exist via an in-process batch lookup before publishing
	productIDs := make([]string, len(cart.Items))
	for i, it := range cart.Items {
		productIDs[i] = it.ProductID
	}

	invalid, err := cc.validateProductsBatch(ctx, productIDs)
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
	correlationID := strings.TrimSpace(c.GetHeader("X-Correlation-ID"))
	if correlationID == "" {
		correlationID = uuid.NewString()
	}

	var reqBody struct {
		PromoCode       string                  `json:"promo_code"`
		ShippingAddress *models.ShippingAddress `json:"shipping_address"`
		Contact         *struct {
			Email string `json:"email"`
			Phone string `json:"phone"`
		} `json:"contact"`
		StoreCreditAmount int64 `json:"store_credit_amount"`
	}
	_ = c.ShouldBindJSON(&reqBody)
	couponCode := cart.CouponCode
	if couponCode == "" && reqBody.PromoCode != "" {
		couponCode = strings.TrimSpace(reqBody.PromoCode)
	}

	customerEmail := ""
	if reqBody.Contact != nil && reqBody.Contact.Email != "" {
		customerEmail = strings.TrimSpace(reqBody.Contact.Email)
	} else if reqBody.ShippingAddress != nil && reqBody.ShippingAddress.Email != "" {
		customerEmail = strings.TrimSpace(reqBody.ShippingAddress.Email)
	}

	// Build SNS payload
	event := buildCheckoutEvent(userID, cart, orderID, scopedIdemKey, correlationID, couponCode)
	event.StoreCreditAmount = reqBody.StoreCreditAmount
	event.StoreID = c.GetHeader("X-Store-ID")
	event.Email = customerEmail
	event.ShippingAddress = reqBody.ShippingAddress

	eventBytes, err := json.Marshal(event)
	if err != nil {
		zap.L().Error("failed to marshal event", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build checkout event"})
		return
	}
	topicArn := os.Getenv("ORDER_SNS_TOPIC_ARN")
	if topicArn == "" {
		topicArn = "arn:aws:sns:eu-west-2:000000000000:order-events"
	}

	if err := cc.SNSClient.Publish(ctx, topicArn, eventBytes); err != nil {
		zap.L().Error("[Checkout] Failed to send SNS event", zap.String("userID", userID), zap.String("topic", topicArn), zap.String("correlation_id", correlationID), zap.Error(err))

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to publish checkout event"})
		return
	}

	// Only persist idempotency mapping AFTER SNS publish succeeds
	// This prevents caching failed orders
	if scopedIdemKey != "" {
		if err := cc.Repo.SetIdempotency(ctx, scopedIdemKey, orderID, cc.CartTTL); err != nil {
			zap.L().Warn("[Checkout] Failed to persist idempotency key", zap.String("orderID", orderID), zap.Error(err))
			// Non-fatal: order was created and published, so continue
		}
	}

	c.JSON(http.StatusOK, gin.H{"order_id": orderID, "status": "PENDING"})
}

func buildCheckoutEvent(userID string, cart *models.Cart, orderID, idempotencyKey, correlationID string, couponCodes ...string) models.CheckoutEvent {
	couponCode := cart.CouponCode
	if len(couponCodes) > 0 && couponCodes[0] != "" {
		couponCode = couponCodes[0]
	}
	return models.CheckoutEvent{
		Event:          "checkout.requested",
		UserID:         userID,
		Items:          cart.Items,
		IdempotencyKey: idempotencyKey,
		Timestamp:      time.Now(),
		OrderID:        orderID,
		CouponCode:     couponCode,
		CorrelationID:  correlationID,
	}
}

// validateProductsBatch validates all given product IDs with a single
// in-process lookup, returning the IDs that are missing/invalid.
func (cc *CartController) validateProductsBatch(ctx context.Context, productIDs []string) ([]string, error) {
	if cc.Validator == nil {
		return nil, errValidatorUnavailable
	}

	ids := make([]uuid.UUID, 0, len(productIDs))
	invalid := make([]string, 0)
	seen := make(map[string]bool, len(productIDs))
	for _, raw := range productIDs {
		if seen[raw] {
			continue
		}
		seen[raw] = true
		id, err := uuid.Parse(raw)
		if err != nil {
			invalid = append(invalid, raw)
			continue
		}
		ids = append(ids, id)
	}

	found, err := cc.Validator.GetProductsInternal(ctx, ids)
	if err != nil {
		return nil, err
	}

	foundIDs := make(map[string]bool, len(found))
	for _, p := range found {
		foundIDs[p.ID.String()] = true
	}
	for _, id := range ids {
		if !foundIDs[id.String()] {
			invalid = append(invalid, id.String())
		}
	}
	return invalid, nil
}
