package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"payment-service/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v80"
	"github.com/yashrajoria/common/events"
	"go.uber.org/zap"
)

// StripeWebhook receives and dispatches Stripe webhook events.
func (pc *PaymentController) StripeWebhook(c *gin.Context) {
	event, err := pc.Stripe.ParseWebhook(c.Request)
	if err != nil {
		pc.Logger.Warn("Stripe webhook signature verification failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook"})
		return
	}

	pc.Logger.Info("Processing Stripe webhook",
		zap.String("event_type", string(event.Type)),
		zap.String("event_id", event.ID),
	)

	rawPayload, _ := json.Marshal(event)

	switch event.Type {
	case "checkout.session.completed":
		pc.handleCheckoutCompleted(event, rawPayload)
	case "payment_intent.succeeded":
		pc.handlePaymentIntentStatus(event, "succeeded", rawPayload)
	case "payment_intent.payment_failed":
		pc.handlePaymentIntentStatus(event, "failed", rawPayload)
	default:
		pc.Logger.Info("Unhandled webhook event type", zap.String("event_type", string(event.Type)))
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

func (pc *PaymentController) handleCheckoutCompleted(event stripe.Event, rawPayload []byte) {
	var sess stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
		pc.Logger.Error("Failed to unmarshal checkout session", zap.Error(err))
		return
	}

	orderID := sess.Metadata["order_id"]
	userID := sess.Metadata["user_id"]
	if orderID == "" || userID == "" {
		pc.Logger.Warn("Missing metadata in checkout session",
			zap.String("session_id", sess.ID),
			zap.Any("metadata", sess.Metadata),
		)
		return
	}

	// Look up by order_id from metadata rather than stripe_payment_id.
	// Using stripe_payment_id causes a race: Stripe fires this webhook before
	// CreateCheckoutSession finishes writing the session ID to the DB.
	orderUUID, err := uuid.Parse(orderID)
	if err != nil {
		pc.Logger.Error("Invalid order_id in checkout session metadata",
			zap.String("order_id", orderID), zap.Error(err))
		return
	}

	payment, err := pc.Repo.GetPaymentByOrderID(context.Background(), orderUUID)
	if err != nil {
		pc.Logger.Error("Payment not found for order_id",
			zap.String("order_id", orderID),
			zap.String("session_id", sess.ID),
			zap.Error(err))
		return
	}

	if terminalStatuses[payment.Status] {
		pc.Logger.Info("Skipping duplicate checkout webhook",
			zap.String("payment_id", payment.Payment_ID.String()),
			zap.String("status", payment.Status),
		)
		return
	}

	now := time.Now()
	// Write stripe_payment_id here too — in case setStripePaymentID in
	// CreateCheckoutSession lost the race against this webhook.
	if err := pc.updatePaymentStatus(payment.OrderID, map[string]interface{}{
		"status":               "succeeded",
		"stripe_event_payload": string(rawPayload),
		"succeeded_at":         &now,
		"stripe_payment_id":    sess.ID,
	}); err != nil {
		pc.Logger.Error("Failed to update payment status",
			zap.String("payment_id", payment.Payment_ID.String()),
			zap.Error(err),
		)
		return
	}
	pc.publishPaymentEvent(models.PaymentEvent{
		Type:      "payment_succeeded",
		OrderID:   orderID,
		UserID:    userID,
		PaymentID: payment.Payment_ID.String(),
		Amount:    payment.Amount,
		Currency:  payment.Currency,
		Timestamp: now.UTC(),
	})
}
func (pc *PaymentController) handlePaymentIntentStatus(event stripe.Event, status string, rawPayload []byte) {
	var pi stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
		pc.Logger.Error("Failed to unmarshal payment intent", zap.Error(err))
		return
	}

	// Primary lookup: direct PaymentIntent flow stores the PI ID as stripe_payment_id.
	payment, err := pc.Repo.GetPaymentByStripeID(context.Background(), pi.ID)
	if err != nil {
		// Fallback: Checkout Session flow stores the Session ID, not the PI ID.
		// Try resolving via order_id from the PaymentIntent metadata.
		orderIDStr := pi.Metadata["order_id"]
		if orderIDStr == "" {
			pc.Logger.Error("Payment not found for PaymentIntent and no order_id in metadata",
				zap.String("payment_intent_id", pi.ID),
				zap.Error(err),
			)
			return
		}
		orderUUID, parseErr := uuid.Parse(orderIDStr)
		if parseErr != nil {
			pc.Logger.Error("Invalid order_id in PaymentIntent metadata",
				zap.String("order_id", orderIDStr), zap.Error(parseErr))
			return
		}
		payment, err = pc.Repo.GetPaymentByOrderID(context.Background(), orderUUID)
		if err != nil {
			pc.Logger.Error("Payment not found for PaymentIntent",
				zap.String("payment_intent_id", pi.ID),
				zap.String("order_id", orderIDStr),
				zap.Error(err),
			)
			return
		}
	}

	if terminalStatuses[payment.Status] {
		pc.Logger.Info("Skipping duplicate payment webhook",
			zap.String("payment_id", payment.Payment_ID.String()),
			zap.String("status", payment.Status),
		)
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":               status,
		"stripe_event_payload": string(rawPayload),
	}
	switch status {
	case "succeeded":
		updates["succeeded_at"] = &now
	case "failed":
		updates["failed_at"] = &now
	}

	if err := pc.updatePaymentStatus(payment.OrderID, updates); err != nil {
		pc.Logger.Error("Failed to update payment status",
			zap.String("payment_id", payment.Payment_ID.String()),
			zap.Error(err),
		)
		return
	}

	pc.publishPaymentEvent(models.PaymentEvent{
		Type:      "payment_" + status,
		OrderID:   payment.OrderID.String(),
		UserID:    payment.UserID.String(),
		PaymentID: payment.Payment_ID.String(),
		Amount:    payment.Amount,
		Currency:  payment.Currency,
		Timestamp: now.UTC(),
	})

	if status == "failed" {
		email := pi.ReceiptEmail
		notificationEvent := events.NewPaymentFailedEvent(
			payment.UserID.String(),
			email,
			"",
			"",
			payment.OrderID.String(),
			float64(payment.Amount),
		)
		payload, err := json.Marshal(notificationEvent)
		if err != nil {
			pc.Logger.Warn("Failed to marshal payment_failed notification event", zap.Error(err))
			return
		}
		if err := pc.SNS.Publish(context.Background(), pc.NotificationTopicArn, payload); err != nil {
			pc.Logger.Warn("Failed to publish payment_failed notification event", zap.Error(err))
		}
	}
}
