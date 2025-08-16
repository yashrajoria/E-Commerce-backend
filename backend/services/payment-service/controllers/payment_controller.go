package controllers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"payment-service/database"
	"payment-service/kafka"
	"payment-service/middleware"
	"payment-service/models"
	"payment-service/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v80"
)

type PaymentController struct {
	Stripe *services.StripeService
	Kafka  *kafka.PaymentEventProducer
}

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
		StripePaymentID: piID,
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

	switch event.Type {
	case "payment_intent.succeeded":
		pc.handlePaymentStatus(event, "succeeded", eventBytes)

	case "payment_intent.payment_failed":
		pc.handlePaymentStatus(event, "failed", eventBytes)
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

// Updates DB + publishes standardized Kafka events
func (pc *PaymentController) handlePaymentStatus(event stripe.Event, status string, payload []byte) {
	var pi stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
		return
	}

	var payment models.Payment
	if err := database.DB.Where("stripe_payment_id = ?", pi.ID).First(&payment).Error; err != nil {
		return // not found
	}

	if payment.Status == "succeeded" || payment.Status == "failed" {
		return // already final
	}

	// Update DB
	database.DB.Model(&payment).Updates(map[string]interface{}{
		"status":               status,
		"stripe_event_payload": string(payload),
		"updated_at":           time.Now(),
	})

	// Build and send Kafka event
	eventMsg := models.PaymentEvent{
		Type:      "payment_" + status,
		OrderID:   payment.OrderID.String(),
		UserID:    payment.UserID.String(),
		PaymentID: payment.ID.String(),
		Amount:    payment.Amount,
		Currency:  payment.Currency,
		Timestamp: time.Now().UTC(),
	}

	if err := pc.Kafka.SendPaymentEvent(eventMsg); err != nil {
		// logging only, avoid crashing webhook
	}
}
