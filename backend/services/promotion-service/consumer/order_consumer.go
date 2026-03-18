package consumer

import (
	"context"
	"encoding/json"
	"log"
	"promotion-service/services"

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
	log.Println("[PromotionService][OrderCreatedConsumer] Starting order_created queue consumer")

	err := c.sqsConsumer.StartPolling(ctx, func(ctx context.Context, body string) error {
		return c.handleMessage(ctx, body)
	})
	if err != nil && err != context.Canceled {
		log.Printf("❌ [PromotionService][OrderCreatedConsumer] polling error: %v", err)
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
		log.Printf("❌ [PromotionService] invalid JSON: %v", err)
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

	log.Printf("📥 [PromotionService] Processing coupon usage for code=%s order_id=%v", couponCode, evt.Data["order_id"])

	if err := c.couponService.IncrementCouponUsage(ctx, couponCode); err != nil {
		log.Printf("❌ [PromotionService] Failed to increment usage for code=%s: %v", couponCode, err)
		return err // Retry on DB failures
	}

	log.Printf("✅ [PromotionService] Incremented usage for code=%s", couponCode)
	return nil
}
