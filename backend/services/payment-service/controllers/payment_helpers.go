package controllers

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"payment-service/database"
	"payment-service/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// frontendURL returns the configured frontend base URL, falling back to localhost.
func (pc *PaymentController) frontendURL() string {
	if url := os.Getenv("FRONTEND_URL"); url != "" {
		return url
	}
	return "http://localhost:3000"
}

// respondError logs a warning and writes a JSON error response.
// The status argument should be an http.Status* constant from the caller.
func (pc *PaymentController) respondError(c *gin.Context, status int, msg string, err error) {
	if err != nil {
		pc.Logger.Warn(msg, zap.Error(err))
	}
	c.JSON(status, gin.H{"error": msg})
}

// updatePaymentStatus applies a set of column updates to a payment row by order UUID.
// updated_at is always set automatically.
func (pc *PaymentController) updatePaymentStatus(orderID uuid.UUID, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now()
	return database.DB.Model(&models.Payment{}).Where("order_id = ?", orderID).Updates(updates).Error
}

// setStripePaymentID safely assigns a Stripe session or intent ID to a payment record,
// guarding against conflicts where the same Stripe ID is already assigned to another order.
func (pc *PaymentController) setStripePaymentID(orderID uuid.UUID, stripeID string) error {
	var existing models.Payment
	err := database.DB.Where("stripe_payment_id = ?", stripeID).First(&existing).Error

	if err == gorm.ErrRecordNotFound {
		// No conflict — safe to assign.
		return database.DB.Model(&models.Payment{}).
			Where("order_id = ?", orderID).
			Update("stripe_payment_id", stripeID).Error
	}

	if err != nil {
		return err
	}

	// A record with this stripe_payment_id already exists.
	if existing.OrderID != orderID {
		pc.Logger.Warn("Skipping stripe_payment_id: already assigned to a different payment",
			zap.String("stripe_id", stripeID),
			zap.String("conflicting_order_id", existing.OrderID.String()),
		)
		return nil
	}

	// Same record — idempotent re-assignment.
	return database.DB.Model(&models.Payment{}).
		Where("order_id = ?", orderID).
		Update("stripe_payment_id", stripeID).Error
}

// publishPaymentEvent marshals a PaymentEvent and publishes it to SNS.
func (pc *PaymentController) publishPaymentEvent(event models.PaymentEvent) {
	payload, _ := json.Marshal(event)
	if err := pc.SNS.Publish(context.Background(), pc.TopicArn, payload); err != nil {
		pc.Logger.Error("Failed to publish payment event to SNS",
			zap.String("event_type", event.Type),
			zap.String("order_id", event.OrderID),
			zap.Error(err),
		)
		return
	}
	pc.Logger.Info("Payment event published to SNS",
		zap.String("event_type", event.Type),
		zap.String("order_id", event.OrderID),
	)
}
