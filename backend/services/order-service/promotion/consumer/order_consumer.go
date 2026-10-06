package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"order-service/promotion/repository"
	"order-service/promotion/services"

	"github.com/google/uuid"
	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"github.com/yashrajoria/common/events"
)

type OrderCreatedConsumer struct {
	sqsConsumer   *aws_pkg.SQSConsumer
	couponService services.CouponService
}

func NewOrderCreatedConsumer(sqsConsumer *aws_pkg.SQSConsumer, couponService services.CouponService) *OrderCreatedConsumer {
	return &OrderCreatedConsumer{
		sqsConsumer:   sqsConsumer,
		couponService: couponService,
	}
}

func (c *OrderCreatedConsumer) Start(ctx context.Context) {
	log.Println("[order-service][OrderCreatedConsumer] Starting order_created queue consumer")

	err := c.sqsConsumer.StartPolling(ctx, func(ctx context.Context, body string) error {
		return c.handleMessage(ctx, body)
	})
	if err != nil && err != context.Canceled {
		log.Printf("❌ [order-service][OrderCreatedConsumer] polling error: %v", err)
	}
}

func (c *OrderCreatedConsumer) handleMessage(ctx context.Context, body string) error {
	// Try to unwrap SNS envelope if present
	var snsEnvelope struct {
		Message string `json:"Message"`
	}
	if err := json.Unmarshal([]byte(body), &snsEnvelope); err == nil && snsEnvelope.Message != "" {
		body = snsEnvelope.Message
	}

	var evt events.NotificationEvent
	if err := json.Unmarshal([]byte(body), &evt); err != nil {
		log.Printf("❌ [order-service] invalid JSON: %v", err)
		return nil
	}

	if evt.EventType != "order_created" {
		return nil
	}

	couponCodeRaw, ok := evt.Data["coupon_code"]
	if !ok || couponCodeRaw == nil {
		return nil
	}

	couponCode, ok := couponCodeRaw.(string)
	if !ok || couponCode == "" {
		return nil
	}

	orderIDStr, _ := evt.Data["order_id"].(string)
	orderID, oerr := uuid.Parse(orderIDStr)
	userID, uerr := uuid.Parse(evt.UserID)
	if oerr != nil || uerr != nil {
		log.Printf("❌ [order-service] coupon usage event has invalid order_id=%q or user_id=%q, dropping", orderIDStr, evt.UserID)
		return nil // data issue, retrying won't help
	}

	log.Printf("📥 [order-service] Processing coupon usage for code=%s order_id=%v", couponCode, evt.Data["order_id"])

	if err := c.couponService.IncrementCouponUsage(ctx, couponCode, orderID, userID); err != nil {
		if errors.Is(err, repository.ErrUsageLimitReached) {
			log.Printf("⚠️ [order-service] OVER-REDEMPTION: Coupon usage limit reached for code=%s. Order %v already paid, acknowledging message.", couponCode, evt.Data["order_id"])
			return nil // Swallowing the error to prevent infinite retries
		}
		log.Printf("❌ [order-service] Failed to increment usage for code=%s: %v", couponCode, err)
		return err // Retry on DB failures
	}

	log.Printf("✅ [order-service] Incremented usage for code=%s", couponCode)
	return nil
}
