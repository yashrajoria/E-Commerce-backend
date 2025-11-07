package controllers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"payment-service/database"
	"payment-service/kafka"
	"payment-service/middleware"
	"payment-service/models"
	"payment-service/repository"
	"payment-service/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v80"
	"github.com/stripe/stripe-go/v80/checkout/session"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type PaymentController struct {
	Stripe *services.StripeService
	Kafka  *kafka.PaymentEventProducer
	Logger *zap.Logger
	Repo   repository.PaymentRepository
}

// --- ADD THIS NEW HANDLER ---
// GetPaymentStatusByOrderID is the polling endpoint for the frontend
func (pc *PaymentController) GetPaymentStatusByOrderID(c *gin.Context) {
	orderIDStr := c.Param("order_id")
	log.Println("order id", orderIDStr)
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		pc.Logger.Warn("Invalid Order ID format", zap.String("order_id", orderIDStr), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order ID format"})
		return
	}

	// Fetch payment state from our local DB
	payment, err := pc.Repo.GetPaymentByOrderID(c.Request.Context(), orderID)

	log.Printf("Fetched payment for order_id=%s: %+v, err=%v", orderIDStr, payment, err)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// This is not an error, it just means the consumer hasn't created the record yet.
			// Tell the frontend it's still pending.
			pc.Logger.Info("Payment record not yet found for order_id", zap.String("order_id", orderIDStr))
			c.JSON(http.StatusNotFound, gin.H{
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

	// We found the record, return its current state
	c.JSON(http.StatusOK, gin.H{
		"order_id":     payment.OrderID,
		"status":       payment.Status,
		"checkout_url": payment.CheckoutURL, // This will be null until status is URL_READY
	})
}

// ---

// Initiates a payment via Stripe and stores it in DB
func (pc *PaymentController) InitiatePayment(c *gin.Context) {
	var req struct {
		OrderID  string `json:"order_id" binding:"required"`
		Amount   int    `json:"amount" binding:"required,min=1"`
		Currency string `json:"currency" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := middleware.GetUserID(c)

	// Create Stripe PaymentIntent
	piID, err := pc.Stripe.CreatePaymentIntent(int64(req.Amount), strings.ToLower(req.Currency))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	payment := models.Payment{
		OrderID:         uuid.MustParse(req.OrderID),
		UserID:          uuid.MustParse(userID),
		Amount:          req.Amount,
		Currency:        strings.ToLower(req.Currency),
		Status:          "pending",
		StripePaymentID: &piID,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := database.DB.Create(&payment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save payment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"payment_intent_id": piID})
}

// Handles Stripe webhooks for payment status updates
func (pc *PaymentController) StripeWebhook(c *gin.Context) {
	event, err := pc.Stripe.ParseWebhook(c.Request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid webhook"})
		return
	}

	eventBytes, _ := json.Marshal(event)
	log.Println("Received Stripe webhook:", event.Type)
	switch event.Type {
	case "checkout.session.completed":
		pc.handleCheckoutCompleted(event, eventBytes)
	case "payment_intent.succeeded":
		pc.handlePaymentStatus(event, "succeeded", eventBytes)

	case "payment_intent.payment_failed":
		pc.handlePaymentStatus(event, "failed", eventBytes)
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

// NEW: Handles Checkout Session completion
func (pc *PaymentController) handleCheckoutCompleted(event stripe.Event, payload []byte) {
	var session stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		pc.Logger.Warn("Failed to unmarshal checkout session", zap.Error(err))
		return
	}

	orderID := session.Metadata["order_id"]
	userID := session.Metadata["user_id"]
	log.Println("Checkout session completed:", "session_id", session.ID, "order_id", orderID, "user_id", userID)
	if orderID == "" || userID == "" {
		pc.Logger.Warn("Missing metadata in checkout session", zap.String("session_id", session.ID))
		return
	}

	// Find payment by CheckoutSession ID (not PaymentIntent ID)
	var payment models.Payment
	if err := database.DB.Where("stripe_payment_id = ?", session.ID).First(&payment).Error; err != nil {
		pc.Logger.Warn("Payment not found for session", zap.String("session_id", session.ID), zap.Error(err))
		return
	}

	// Prevent duplicate processing
	if payment.Status == "succeeded" || payment.Status == "failed" {
		pc.Logger.Info("Duplicate checkout webhook", zap.String("payment_id", payment.Payment_ID.String()))
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
		pc.Logger.Warn("Failed to update payment status", zap.Error(err))
		return
	}

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

	if err := pc.Kafka.SendPaymentEvent(eventMsg); err != nil {
		pc.Logger.Warn("Failed to publish payment event", zap.Error(err))
	} else {
		pc.Logger.Info("Payment succeeded event published", zap.String("order_id", orderID))
	}
}

// Updates DB + publishes standardized Kafka events
func (pc *PaymentController) handlePaymentStatus(event stripe.Event, status string, payload []byte) {
	var pi stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
		pc.Logger.Warn("Payment not found for PaymentIntent", zap.String("payment_intent_id", pi.ID), zap.Error(err))

		return
	}

	var payment models.Payment
	if err := database.DB.Where("stripe_payment_id = ?", pi.ID).First(&payment).Error; err != nil {
		pc.Logger.Warn("Payment not found for PaymentIntent", zap.String("payment_intent_id", pi.ID), zap.Error(err))

		return
	}

	if payment.Status == "succeeded" || payment.Status == "failed" {

		pc.Logger.Info("Duplicate payment webhook notification", zap.String("payment_id", payment.Payment_ID.String()), zap.String("status", payment.Status))

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
		pc.Logger.Warn("Failed to update payment status", zap.String("payment_id", payment.Payment_ID.String()), zap.Error(err))
		return
	}

	eventMsg := models.PaymentEvent{
		Type:      "payment_" + status,
		OrderID:   payment.OrderID.String(),
		UserID:    payment.UserID.String(),
		PaymentID: payment.Payment_ID.String(),
		Amount:    payment.Amount,
		Currency:  payment.Currency,
		Timestamp: time.Now().UTC(),
	}

	if err := pc.Kafka.SendPaymentEvent(eventMsg); err != nil {
		pc.Logger.Warn("Failed to publish Kafka payment event", zap.String("payment_id", payment.Payment_ID.String()), zap.Error(err))
	}
}

func (pc *PaymentController) VerifyPayment(c *gin.Context) {
	var req struct {
		PaymentID string `json:"payment_id" binding:"required"`
		SessionID string `json:"session_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sess, err := session.Get(req.SessionID, nil)
	if err != nil {
		pc.Logger.Error("Error fetching Stripe session", zap.String("session_id", req.SessionID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch Stripe session"})
		return
	}

	log.Print(sess)
	c.JSON(http.StatusOK, gin.H{"status": sess.PaymentStatus})
}
