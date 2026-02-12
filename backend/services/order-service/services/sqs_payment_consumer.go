package services

import (
	"context"
	"encoding/json"
	"log"
	"order-service/models"
	"time"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"gorm.io/gorm"
)

// SQSPaymentConsumer consumes payment events from SQS and updates order status
type SQSPaymentConsumer struct {
	sqsConsumer *aws_pkg.SQSConsumer
	db          *gorm.DB
}

// NewSQSPaymentConsumer creates a new SQS-based payment event consumer
func NewSQSPaymentConsumer(sqsConsumer *aws_pkg.SQSConsumer, db *gorm.DB) *SQSPaymentConsumer {
	return &SQSPaymentConsumer{
		sqsConsumer: sqsConsumer,
		db:          db,
	}
}

// Start begins polling the payment events queue
func (c *SQSPaymentConsumer) Start(ctx context.Context) {
	log.Println("[OrderService][SQSPaymentConsumer] Starting payment events queue consumer")

	err := c.sqsConsumer.StartPolling(ctx, func(ctx context.Context, body string) error {
		return c.handleMessage(ctx, body)
	})
	if err != nil && err != context.Canceled {
		log.Printf("❌ [OrderService][SQSPaymentConsumer] polling error: %v", err)
	}
}

func (c *SQSPaymentConsumer) handleMessage(ctx context.Context, body string) error {
	log.Printf("[DEBUG] Raw payment event: %s", body)

	// Try to unwrap SNS envelope if present
	var snsEnvelope struct {
		Message string `json:"Message"`
	}
	if err := json.Unmarshal([]byte(body), &snsEnvelope); err == nil && snsEnvelope.Message != "" {
		body = snsEnvelope.Message
	}

	var evt models.PaymentEvent
	if err := json.Unmarshal([]byte(body), &evt); err != nil {
		log.Printf("❌ [OrderService][SQSPaymentConsumer] invalid JSON: %v payload=%s", err, body)
		return nil // Don't retry invalid JSON
	}

	if evt.OrderID == "" || evt.Type == "" {
		log.Printf("❌ [OrderService][SQSPaymentConsumer] missing fields: order_id=%q type=%q", evt.OrderID, evt.Type)
		return nil
	}

	log.Printf("ℹ️  [OrderService][SQSPaymentConsumer] received event: order_id=%s type=%s", evt.OrderID, evt.Type)

	now := time.Now()
	switch evt.Type {
	case "payment_succeeded":
		c.updateOrderStatusWithTime(evt.OrderID, "paid", &now, nil)
	case "payment_failed":
		c.updateOrderStatusWithTime(evt.OrderID, "payment_failed", nil, &now)
	case "checkout_session_created":
		log.Printf("ℹ️  [OrderService][SQSPaymentConsumer] checkout session created for order=%s", evt.OrderID)
	case "checkout_session_failed":
		c.updateOrderStatusWithTime(evt.OrderID, "payment_failed", nil, &now)
	default:
		log.Printf("⚠️  [OrderService][SQSPaymentConsumer] unknown event type: %s", evt.Type)
	}

	return nil
}

func (c *SQSPaymentConsumer) updateOrderStatusWithTime(orderID, status string, completedAt, canceledAt *time.Time) {
	updateFields := map[string]interface{}{
		"status": status,
	}
	if completedAt != nil {
		updateFields["completed_at"] = *completedAt
	}
	if canceledAt != nil {
		updateFields["canceled_at"] = *canceledAt
	}

	err := c.db.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.First(&order, "id = ?", orderID).Error; err != nil {
			return err
		}
		if order.Status == status {
			needsUpdate := false
			if completedAt != nil && order.CompletedAt == nil {
				updateFields["completed_at"] = *completedAt
				needsUpdate = true
			}
			if canceledAt != nil && order.CanceledAt == nil {
				updateFields["canceled_at"] = *canceledAt
				needsUpdate = true
			}
			if !needsUpdate {
				log.Printf("ℹ️  [OrderService][SQSPaymentConsumer] order=%s already %s; skipping", orderID, status)
				return nil
			}
		}
		return tx.Model(&order).Updates(updateFields).Error
	})
	if err != nil {
		log.Printf("❌ [OrderService][SQSPaymentConsumer] failed to update order=%s: %v", orderID, err)
	} else {
		log.Printf("✅ [OrderService][SQSPaymentConsumer] order=%s updated to %s", orderID, status)
	}
}
