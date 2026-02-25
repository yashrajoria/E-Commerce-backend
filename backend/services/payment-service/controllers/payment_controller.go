package controllers

import (
	"net/http"
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

// terminalStatuses are payment statuses that should not be overwritten.
var terminalStatuses = map[string]bool{
	"succeeded": true,
	"failed":    true,
}

// PaymentController handles all payment-related HTTP and webhook logic.
type PaymentController struct {
	Stripe   *services.StripeService
	SNS      *aws_pkg.SNSClient
	TopicArn string
	Logger   *zap.Logger
	Repo     repository.PaymentRepository
}

// GetPaymentStatusByOrderID is a polling endpoint for the frontend.
func (pc *PaymentController) GetPaymentStatusByOrderID(c *gin.Context) {
	orderIDStr := c.Param("order_id")

	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		pc.respondError(c, http.StatusBadRequest, "invalid order ID format", err)
		return
	}

	payment, err := pc.Repo.GetPaymentByOrderID(c.Request.Context(), orderID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Payment not yet created — return a synthetic PENDING state.
			c.JSON(http.StatusOK, gin.H{
				"order_id":     orderIDStr,
				"status":       "PENDING",
				"checkout_url": nil,
			})
			return
		}
		pc.Logger.Error("Error fetching payment by order_id", zap.String("order_id", orderIDStr), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"order_id":     payment.OrderID.String(),
		"status":       payment.Status,
		"checkout_url": payment.CheckoutURL,
		"session_id":   payment.StripePaymentID,
	})
}

// CreateCheckoutSession creates a Stripe Checkout Session and stores the URL in the DB.
func (pc *PaymentController) CreateCheckoutSession(c *gin.Context) {
	var req struct {
		OrderID string `json:"order_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		pc.respondError(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	orderUUID, err := uuid.Parse(req.OrderID)
	if err != nil {
		pc.respondError(c, http.StatusBadRequest, "invalid order ID format", err)
		return
	}

	payment, err := pc.Repo.GetPaymentByOrderID(c.Request.Context(), orderUUID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			pc.respondError(c, http.StatusNotFound, "payment record not found", nil)
			return
		}
		pc.Logger.Error("Error fetching payment by order_id", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	amount := int64(payment.Amount)
	if amount <= 0 {
		pc.respondError(c, http.StatusBadRequest, "invalid payment amount", nil)
		return
	}

	currency := payment.Currency
	if currency == "" {
		currency = "usd"
	}

	frontend := pc.frontendURL()
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
		SuccessURL: stripe.String(frontend + "/payment/success?session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:  stripe.String(frontend + "/payment/cancel"),
		Metadata: map[string]string{
			"order_id": req.OrderID,
			"user_id":  payment.UserID.String(),
		},
	}

	sess, err := session.New(params)
	if err != nil {
		pc.Logger.Error("Failed to create Stripe checkout session", zap.String("order_id", req.OrderID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create checkout session"})
		return
	}

	// Persist checkout URL and mark URL_READY.
	if err := pc.updatePaymentStatus(orderUUID, map[string]interface{}{
		"checkout_url": sess.URL,
		"status":       "URL_READY",
	}); err != nil {
		pc.Logger.Warn("Failed to update payment with checkout URL", zap.String("order_id", req.OrderID), zap.Error(err))
		// Non-fatal: still return the URL to the caller.
	}

	if err := pc.setStripePaymentID(orderUUID, sess.ID); err != nil {
		pc.Logger.Warn("Failed to set stripe_payment_id", zap.String("order_id", req.OrderID), zap.Error(err))
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id":   sess.ID,
		"checkout_url": sess.URL,
	})
}

// InitiatePayment creates a Stripe PaymentIntent and persists the record.
// Deprecated: prefer CreateCheckoutSession.
func (pc *PaymentController) InitiatePayment(c *gin.Context) {
	var req struct {
		OrderID  string `json:"order_id" binding:"required"`
		Amount   int    `json:"amount" binding:"required,min=1"`
		Currency string `json:"currency" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		pc.respondError(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	userID := middleware.GetUserID(c)

	pi, err := pc.Stripe.CreatePaymentIntent(int64(req.Amount), strings.ToLower(req.Currency))
	if err != nil {
		pc.Logger.Error("Failed to create payment intent", zap.String("order_id", req.OrderID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save payment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"payment_intent_id": pi.ID})
}

// VerifyPayment cross-checks a session ID against Stripe and returns live status.
func (pc *PaymentController) VerifyPayment(c *gin.Context) {
	var req struct {
		PaymentID string `json:"payment_id" binding:"required"`
		SessionID string `json:"session_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		pc.respondError(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	sess, err := session.Get(req.SessionID, nil)
	if err != nil {
		pc.Logger.Error("Error fetching Stripe session", zap.String("session_id", req.SessionID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch Stripe session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"payment_status": sess.PaymentStatus,
		"session_status": sess.Status,
	})
}
