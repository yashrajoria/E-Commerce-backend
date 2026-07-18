package controllers

import (
	"context"
	"encoding/json"
	"fmt"
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

	// Fulfill first, then record event_id. Claiming before fulfill permanently
	// drops Stripe retries if mid-flight processing fails (Context7 / Stripe guidance:
	// fulfillment must be safely re-runnable; terminal payment status is the concurrency guard).
	var fulfillErr error
	switch event.Type {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
		fulfillErr = pc.handleCheckoutCompleted(event, rawPayload)
	case "payment_intent.succeeded":
		fulfillErr = pc.handlePaymentIntentStatus(event, "succeeded", rawPayload)
	case "payment_intent.payment_failed":
		fulfillErr = pc.handlePaymentIntentStatus(event, "failed", rawPayload)
	default:
		pc.Logger.Info("Unhandled webhook event type", zap.String("event_type", string(event.Type)))
	}
	if fulfillErr != nil {
		pc.Logger.Error("Stripe webhook fulfillment failed",
			zap.String("event_id", event.ID),
			zap.String("event_type", string(event.Type)),
			zap.Error(fulfillErr),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "webhook processing failed"})
		return
	}

	if _, err := pc.Repo.MarkStripeEventProcessed(c.Request.Context(), event.ID, string(event.Type)); err != nil {
		// Fulfillment already succeeded; log and still ack so Stripe does not loop forever.
		pc.Logger.Warn("Failed to record Stripe event id after fulfill",
			zap.String("event_id", event.ID), zap.Error(err))
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

func (pc *PaymentController) handleCheckoutCompleted(event stripe.Event, rawPayload []byte) error {
	var sess stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
		pc.Logger.Error("Failed to unmarshal checkout session", zap.Error(err))
		return fmt.Errorf("unmarshal checkout session: %w", err)
	}

	orderID := sess.Metadata["order_id"]
	userID := sess.Metadata["user_id"]
	if orderID == "" || userID == "" {
		pc.Logger.Warn("Missing metadata in checkout session",
			zap.String("session_id", sess.ID),
			zap.Any("metadata", sess.Metadata),
		)
		// Acknowledge without retry — missing metadata will not self-heal.
		return nil
	}

	// Look up by order_id from metadata rather than stripe_payment_id.
	// Using stripe_payment_id causes a race: Stripe fires this webhook before
	// CreateCheckoutSession finishes writing the session ID to the DB.
	orderUUID, err := uuid.Parse(orderID)
	if err != nil {
		pc.Logger.Error("Invalid order_id in checkout session metadata",
			zap.String("order_id", orderID), zap.Error(err))
		return nil
	}

	payment, err := pc.Repo.GetPaymentByOrderID(context.Background(), orderUUID)
	if err != nil {
		pc.Logger.Error("Payment not found for order_id",
			zap.String("order_id", orderID),
			zap.String("session_id", sess.ID),
			zap.Error(err))
		return fmt.Errorf("payment not found for order %s: %w", orderID, err)
	}

	if terminalStatuses[payment.Status] {
		pc.Logger.Info("Skipping duplicate checkout webhook",
			zap.String("payment_id", payment.Payment_ID.String()),
			zap.String("status", payment.Status),
		)
		return nil
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
		return fmt.Errorf("update payment status: %w", err)
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
	return nil
}

func (pc *PaymentController) handlePaymentIntentStatus(event stripe.Event, status string, rawPayload []byte) error {
	var pi stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
		pc.Logger.Error("Failed to unmarshal payment intent", zap.Error(err))
		return fmt.Errorf("unmarshal payment intent: %w", err)
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
			return fmt.Errorf("payment not found for payment_intent %s: %w", pi.ID, err)
		}
		orderUUID, parseErr := uuid.Parse(orderIDStr)
		if parseErr != nil {
			pc.Logger.Error("Invalid order_id in PaymentIntent metadata",
				zap.String("order_id", orderIDStr), zap.Error(parseErr))
			return nil
		}
		payment, err = pc.Repo.GetPaymentByOrderID(context.Background(), orderUUID)
		if err != nil {
			pc.Logger.Error("Payment not found for PaymentIntent",
				zap.String("payment_intent_id", pi.ID),
				zap.String("order_id", orderIDStr),
				zap.Error(err),
			)
			return fmt.Errorf("payment not found for payment_intent %s: %w", pi.ID, err)
		}
	}

	if terminalStatuses[payment.Status] {
		pc.Logger.Info("Skipping duplicate payment webhook",
			zap.String("payment_id", payment.Payment_ID.String()),
			zap.String("status", payment.Status),
		)
		return nil
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
		return fmt.Errorf("update payment status: %w", err)
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
			return nil
		}
		if err := pc.SNS.Publish(context.Background(), pc.NotificationTopicArn, payload); err != nil {
			pc.Logger.Warn("Failed to publish payment_failed notification event", zap.Error(err))
		}
	}
	return nil
}
