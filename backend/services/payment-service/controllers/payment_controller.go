package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"payment-service/database"
	"payment-service/middleware"
	"payment-service/models"
	"payment-service/repository"
	"payment-service/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v80"
	"github.com/stripe/stripe-go/v80/checkout/session"
	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type PaymentController struct {
	Stripe   *services.StripeService
	SNS      *aws_pkg.SNSClient
	TopicArn string
	Logger   *zap.Logger
	Repo     repository.PaymentRepository
}

// GetPaymentStatusByOrderID is the polling endpoint for the frontend
func (pc *PaymentController) GetPaymentStatusByOrderID(c *gin.Context) {
	orderIDStr := c.Param("order_id")
	pc.Logger.Info("Fetching payment status", zap.String("order_id", orderIDStr))

	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		pc.Logger.Warn("Invalid Order ID format", zap.String("order_id", orderIDStr), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order ID format"})
		return
	}

	// Fetch payment state from our local DB
	payment, err := pc.Repo.GetPaymentByOrderID(c.Request.Context(), orderID)

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			pc.Logger.Info("Payment record not yet found for order_id", zap.String("order_id", orderIDStr))
			c.JSON(http.StatusOK, gin.H{
				"order_id":     orderIDStr,
				"status":       "PENDING",
				"checkout_url": nil,
			})
			return
		}
		// This is a real database error
		pc.Logger.Error("Error fetching payment by order_id", zap.String("order_id", orderIDStr), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	pc.Logger.Info("Payment record found",
		zap.String("order_id", orderIDStr),
		zap.String("status", payment.Status),
		zap.String("payment_id", payment.Payment_ID.String()),
	)

	// We found the record, return its current state
	c.JSON(http.StatusOK, gin.H{
		"order_id":     payment.OrderID.String(),
		"status":       payment.Status,
		"checkout_url": payment.CheckoutURL, // This will be populated when status is URL_READY
	})
}

// CreateCheckoutSession creates a Stripe Checkout Session and stores the URL in DB
func (pc *PaymentController) CreateCheckoutSession(c *gin.Context) {
	var req struct {
		OrderID string `json:"order_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		pc.Logger.Warn("Invalid request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	orderUUID, err := uuid.Parse(req.OrderID)
	if err != nil {
		pc.Logger.Warn("Invalid order ID format", zap.String("order_id", req.OrderID), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order ID format"})
		return
	}

	// Look up existing payment record for this order
	payment, err := pc.Repo.GetPaymentByOrderID(c.Request.Context(), orderUUID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			pc.Logger.Warn("Payment record not found for order", zap.String("order_id", req.OrderID))
			c.JSON(http.StatusNotFound, gin.H{"error": "payment record not found"})
			return
		}
		pc.Logger.Error("Error fetching payment by order_id", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Derive amount and currency from payment record
	amount := int64(payment.Amount)
	if amount <= 0 {
		pc.Logger.Warn("Payment amount is zero or invalid", zap.String("order_id", req.OrderID))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payment amount"})
		return
	}
	currency := payment.Currency
	if currency == "" {
		currency = "usd"
	}

	// Build success/cancel URLs from FRONTEND_URL env
	frontend := os.Getenv("FRONTEND_URL")
	if frontend == "" {
		frontend = "http://localhost:3000"
	}
	successURL := frontend + "/payment/success?session_id={CHECKOUT_SESSION_ID}"
	cancelURL := frontend + "/payment/cancel"

	pc.Logger.Info("Creating checkout session (server-populated fields)",
		zap.String("order_id", req.OrderID),
		zap.Int64("amount", amount),
		zap.String("currency", currency),
		zap.String("success_url", successURL),
	)

	// Create Stripe Checkout Session
	params := &stripe.CheckoutSessionParams{
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency: stripe.String(strings.ToLower(currency)),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String("Order #" + req.OrderID),
					},
					UnitAmount: stripe.Int64(amount),
				},
				Quantity: stripe.Int64(1),
			},
		},
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
		Metadata: map[string]string{
			"order_id": req.OrderID,
			"user_id":  payment.UserID.String(),
		},
	}

	checkoutSession, err := session.New(params)
	if err != nil {
		pc.Logger.Error("Failed to create Stripe checkout session",
			zap.String("order_id", req.OrderID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create checkout session"})
		return
	}

	pc.Logger.Info("Stripe checkout session created",
		zap.String("session_id", checkoutSession.ID),
		zap.String("checkout_url", checkoutSession.URL),
		zap.String("order_id", req.OrderID),
	)

	// Update payment record with checkout URL and status first
	updates := map[string]interface{}{
		"checkout_url": checkoutSession.URL,
		"status":       "URL_READY",
		"updated_at":   time.Now(),
	}

	if err := database.DB.Model(&models.Payment{}).Where("order_id = ?", orderUUID).Updates(updates).Error; err != nil {
		pc.Logger.Warn("Failed to update payment with checkout URL",
			zap.String("order_id", req.OrderID),
			zap.Error(err),
		)
		// still return URL to caller even if DB update failed
	}

	// Attempt to set `stripe_payment_id` only if it won't conflict with another record
	var existing models.Payment
	if err := database.DB.Where("stripe_payment_id = ?", checkoutSession.ID).First(&existing).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// safe to set
			if err := database.DB.Model(&models.Payment{}).Where("order_id = ?", orderUUID).Update("stripe_payment_id", checkoutSession.ID).Error; err != nil {
				pc.Logger.Warn("Failed to set stripe_payment_id",
					zap.String("order_id", req.OrderID),
					zap.Error(err),
				)
			}
		} else {
			pc.Logger.Warn("Error checking existing stripe_payment_id",
				zap.Error(err),
			)
		}
	} else {
		// Found an existing record with this stripe_payment_id
		if existing.OrderID == orderUUID {
			// same record, ensure stripe_payment_id is set (idempotent)
			if err := database.DB.Model(&models.Payment{}).Where("order_id = ?", orderUUID).Update("stripe_payment_id", checkoutSession.ID).Error; err != nil {
				pc.Logger.Warn("Failed to ensure stripe_payment_id on same payment",
					zap.String("order_id", req.OrderID),
					zap.Error(err),
				)
			}
		} else {
			pc.Logger.Warn("Skipping setting stripe_payment_id because it already exists on a different payment",
				zap.String("order_id", req.OrderID),
				zap.String("conflicting_order_id", existing.OrderID.String()),
			)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id":   checkoutSession.ID,
		"checkout_url": checkoutSession.URL,
	})
}

// Initiates a payment via Stripe PaymentIntent (legacy method - consider deprecating)
func (pc *PaymentController) InitiatePayment(c *gin.Context) {
	var req struct {
		OrderID  string `json:"order_id" binding:"required"`
		Amount   int    `json:"amount" binding:"required,min=1"`
		Currency string `json:"currency" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		pc.Logger.Warn("Invalid request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := middleware.GetUserID(c)
	pc.Logger.Info("Initiating payment",
		zap.String("order_id", req.OrderID),
		zap.String("user_id", userID),
		zap.Int("amount", req.Amount),
	)

	// Create Stripe PaymentIntent
	pi, err := pc.Stripe.CreatePaymentIntent(int64(req.Amount), strings.ToLower(req.Currency))
	if err != nil {
		pc.Logger.Error("Failed to create payment intent",
			zap.String("order_id", req.OrderID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	pc.Logger.Info("Payment intent created", zap.String("payment_intent_id", pi.ID))

	payment := models.Payment{
		OrderID:         uuid.MustParse(req.OrderID),
		UserID:          uuid.MustParse(userID),
		Amount:          req.Amount,
		Currency:        strings.ToLower(req.Currency),
		Status:          "pending",
		StripePaymentID: &pi.ID,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := database.DB.Create(&payment).Error; err != nil {
		pc.Logger.Error("Failed to save payment", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save payment"})
		return
	}

	pc.Logger.Info("Payment record saved", zap.String("payment_id", payment.Payment_ID.String()))

	c.JSON(http.StatusOK, gin.H{"payment_intent_id": pi.ID})
}

// Handles Stripe webhooks for payment status updates
func (pc *PaymentController) StripeWebhook(c *gin.Context) {
	pc.Logger.Info("Stripe webhook received",
		zap.String("path", c.FullPath()),
		zap.Bool("has_signature", c.GetHeader("Stripe-Signature") != ""),
	)

	event, err := pc.Stripe.ParseWebhook(c.Request)
	if err != nil {
		pc.Logger.Warn("Stripe webhook signature verification failed",
			zap.Error(err),
			zap.String("stripe_signature", c.GetHeader("Stripe-Signature")),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid webhook"})
		return
	}

	eventBytes, _ := json.Marshal(event)
	pc.Logger.Info("Processing Stripe webhook event",
		zap.String("event_type", string(event.Type)),
		zap.String("event_id", event.ID),
	)

	switch event.Type {
	case "checkout.session.completed":
		pc.handleCheckoutCompleted(event, eventBytes)
	case "payment_intent.succeeded":
		pc.handlePaymentStatus(event, "succeeded", eventBytes)
	case "payment_intent.payment_failed":
		pc.handlePaymentStatus(event, "failed", eventBytes)
	default:
		pc.Logger.Info("Unhandled webhook event type", zap.String("event_type", string(event.Type)))
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

// Handles Checkout Session completion
func (pc *PaymentController) handleCheckoutCompleted(event stripe.Event, payload []byte) {
	var session stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		pc.Logger.Error("Failed to unmarshal checkout session", zap.Error(err))
		return
	}

	orderID := session.Metadata["order_id"]
	userID := session.Metadata["user_id"]

	pc.Logger.Info("Processing checkout session completion",
		zap.String("session_id", session.ID),
		zap.String("order_id", orderID),
		zap.String("user_id", userID),
		zap.String("payment_status", string(session.PaymentStatus)),
	)

	if orderID == "" || userID == "" {
		pc.Logger.Warn("Missing metadata in checkout session",
			zap.String("session_id", session.ID),
			zap.Any("metadata", session.Metadata),
		)
		return
	}

	// Find payment by CheckoutSession ID
	var payment models.Payment
	if err := database.DB.Where("stripe_payment_id = ?", session.ID).First(&payment).Error; err != nil {
		pc.Logger.Error("Payment not found for session",
			zap.String("session_id", session.ID),
			zap.Error(err),
		)
		return
	}

	pc.Logger.Info("Found payment record for session",
		zap.String("payment_id", payment.Payment_ID.String()),
		zap.String("current_status", payment.Status),
	)

	// Prevent duplicate processing
	if payment.Status == "succeeded" || payment.Status == "failed" {
		pc.Logger.Info("Duplicate checkout webhook - payment already processed",
			zap.String("payment_id", payment.Payment_ID.String()),
			zap.String("status", payment.Status),
		)
		return
	}

	updates := map[string]interface{}{
		"status":               "succeeded",
		"stripe_event_payload": string(payload),
		"updated_at":           time.Now(),
	}
	now := time.Now()
	updates["succeeded_at"] = &now

	if err := database.DB.Model(&payment).Updates(updates).Error; err != nil {
		pc.Logger.Error("Failed to update payment status",
			zap.String("payment_id", payment.Payment_ID.String()),
			zap.Error(err),
		)
		return
	}

	pc.Logger.Info("Payment status updated to succeeded",
		zap.String("payment_id", payment.Payment_ID.String()),
		zap.String("order_id", orderID),
	)

	// Publish payment success event
	eventMsg := models.PaymentEvent{
		Type:      "payment_succeeded",
		OrderID:   orderID,
		UserID:    userID,
		PaymentID: payment.Payment_ID.String(),
		Amount:    payment.Amount,
		Currency:  payment.Currency,
		Timestamp: time.Now().UTC(),
	}

	eventBytes, _ := json.Marshal(eventMsg)
	if err := pc.SNS.Publish(context.Background(), pc.TopicArn, eventBytes); err != nil {
		pc.Logger.Error("Failed to publish payment event to SNS",
			zap.String("order_id", orderID),
			zap.Error(err),
		)
	} else {
		pc.Logger.Info("Payment succeeded event published to SNS",
			zap.String("order_id", orderID),
			zap.String("payment_id", payment.Payment_ID.String()),
		)
	}
}

// Updates DB + publishes standardized SNS events
func (pc *PaymentController) handlePaymentStatus(event stripe.Event, status string, payload []byte) {
	var pi stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
		pc.Logger.Error("Failed to unmarshal payment intent", zap.Error(err))
		return
	}

	pc.Logger.Info("Processing payment intent status",
		zap.String("payment_intent_id", pi.ID),
		zap.String("status", status),
	)

	var payment models.Payment
	if err := database.DB.Where("stripe_payment_id = ?", pi.ID).First(&payment).Error; err != nil {
		pc.Logger.Error("Payment not found for PaymentIntent",
			zap.String("payment_intent_id", pi.ID),
			zap.Error(err),
		)
		return
	}

	pc.Logger.Info("Found payment record for payment intent",
		zap.String("payment_id", payment.Payment_ID.String()),
		zap.String("current_status", payment.Status),
	)

	if payment.Status == "succeeded" || payment.Status == "failed" {
		pc.Logger.Info("Duplicate payment webhook notification - already processed",
			zap.String("payment_id", payment.Payment_ID.String()),
			zap.String("status", payment.Status),
		)
		return
	}

	updates := map[string]interface{}{
		"status":               status,
		"stripe_event_payload": string(payload),
		"updated_at":           time.Now(),
	}

	now := time.Now()
	switch status {
	case "succeeded":
		updates["succeeded_at"] = &now
	case "failed":
		updates["failed_at"] = &now
	}

	if err := database.DB.Model(&payment).Updates(updates).Error; err != nil {
		pc.Logger.Error("Failed to update payment status",
			zap.String("payment_id", payment.Payment_ID.String()),
			zap.String("new_status", status),
			zap.Error(err),
		)
		return
	}

	pc.Logger.Info("Payment status updated",
		zap.String("payment_id", payment.Payment_ID.String()),
		zap.String("new_status", status),
	)

	eventMsg := models.PaymentEvent{
		Type:      "payment_" + status,
		OrderID:   payment.OrderID.String(),
		UserID:    payment.UserID.String(),
		PaymentID: payment.Payment_ID.String(),
		Amount:    payment.Amount,
		Currency:  payment.Currency,
		Timestamp: time.Now().UTC(),
	}

	eventBytes, _ := json.Marshal(eventMsg)
	if err := pc.SNS.Publish(context.Background(), pc.TopicArn, eventBytes); err != nil {
		pc.Logger.Error("Failed to publish payment event to SNS",
			zap.String("payment_id", payment.Payment_ID.String()),
			zap.String("event_type", eventMsg.Type),
			zap.Error(err),
		)
	} else {
		pc.Logger.Info("Payment event published to SNS",
			zap.String("payment_id", payment.Payment_ID.String()),
			zap.String("event_type", eventMsg.Type),
		)
	}
}

func (pc *PaymentController) VerifyPayment(c *gin.Context) {
	var req struct {
		PaymentID string `json:"payment_id" binding:"required"`
		SessionID string `json:"session_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		pc.Logger.Warn("Invalid request body for verify payment", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pc.Logger.Info("Verifying payment",
		zap.String("payment_id", req.PaymentID),
		zap.String("session_id", req.SessionID),
	)

	sess, err := session.Get(req.SessionID, nil)
	if err != nil {
		pc.Logger.Error("Error fetching Stripe session",
			zap.String("session_id", req.SessionID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch Stripe session"})
		return
	}

	pc.Logger.Info("Stripe session retrieved",
		zap.String("session_id", req.SessionID),
		zap.String("payment_status", string(sess.PaymentStatus)),
		zap.String("status", string(sess.Status)),
	)

	c.JSON(http.StatusOK, gin.H{
		"payment_status": sess.PaymentStatus,
		"session_status": sess.Status,
	})
}
