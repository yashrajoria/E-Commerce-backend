package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/yashrajoria/common/events"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"order-service/models"
)

type PriceDropRepository interface {
	FindEligibleOrderItems(ctx context.Context, productID uuid.UUID, cutoff time.Time) ([]models.OrderItem, error)
	RecordRefundOutboxEvents(ctx context.Context, events []models.OutboxEvent) error
}

type GormPriceDropRepository struct {
	db *gorm.DB
}

func NewGormPriceDropRepository(db *gorm.DB) *GormPriceDropRepository {
	return &GormPriceDropRepository{db: db}
}

func (r *GormPriceDropRepository) FindEligibleOrderItems(ctx context.Context, productID uuid.UUID, cutoff time.Time) ([]models.OrderItem, error) {
	var orderItems []models.OrderItem
	err := r.db.WithContext(ctx).
		Joins("JOIN orders ON orders.id = order_items.order_id").
		Where("order_items.product_id = ? AND orders.status IN (?) AND orders.created_at >= ?",
			productID, []string{"paid", "completed"}, cutoff).
		Find(&orderItems).Error
	return orderItems, err
}

func (r *GormPriceDropRepository) RecordRefundOutboxEvents(ctx context.Context, events []models.OutboxEvent) error {
	if len(events) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range events {
			if err := tx.Create(&events[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

type PriceDropService interface {
	ProcessPriceDrop(ctx context.Context, event events.ProductPriceDecreasedEvent) (*PriceDropResult, error)
}

type PriceDropResult struct {
	EligibleOrdersCount int     `json:"eligible_orders_count"`
	TotalRefundAmount   float64 `json:"total_refund_amount"`
	OutboxEventsCreated int     `json:"outbox_events_created"`
}

type priceDropServiceImpl struct {
	repo                 PriceDropRepository
	notificationTopicARN string
	logger               *zap.Logger
}

func NewPriceDropService(repo PriceDropRepository, notificationTopicARN string, logger *zap.Logger) PriceDropService {
	return &priceDropServiceImpl{
		repo:                 repo,
		notificationTopicARN: notificationTopicARN,
		logger:               logger,
	}
}

func (s *priceDropServiceImpl) ProcessPriceDrop(ctx context.Context, event events.ProductPriceDecreasedEvent) (*PriceDropResult, error) {
	if event.DeltaPrice <= 0 {
		return &PriceDropResult{}, nil
	}

	prodUUID, err := uuid.Parse(event.ProductID)
	if err != nil {
		return nil, fmt.Errorf("invalid product uuid in price drop event: %w", err)
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -14)
	orderItems, err := s.repo.FindEligibleOrderItems(ctx, prodUUID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to query eligible orders for price drop refund: %w", err)
	}

	if len(orderItems) == 0 {
		s.logger.Info("No eligible orders within 14 days found for price drop refund",
			zap.String("product_id", event.ProductID),
			zap.Float64("delta_price", event.DeltaPrice),
		)
		return &PriceDropResult{EligibleOrdersCount: 0}, nil
	}

	deltaCents := int(event.DeltaPrice * 100)
	var totalRefundCents int
	outboxEvents := make([]models.OutboxEvent, 0, len(orderItems))

	for _, item := range orderItems {
		refundForLineItem := deltaCents * item.Quantity
		totalRefundCents += refundForLineItem

		payloadMap := map[string]interface{}{
			"order_id":            item.OrderID.String(),
			"product_id":          item.ProductID.String(),
			"quantity":            item.Quantity,
			"refund_amount_cents": refundForLineItem,
			"old_price":           event.OldPrice,
			"new_price":           event.NewPrice,
			"delta_price":         event.DeltaPrice,
			"reason":              "14-Day Price Drop Guarantee automatic refund",
			"timestamp":           time.Now().UTC().Format(time.RFC3339),
		}

		payloadBytes, err := json.Marshal(payloadMap)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal refund payload: %w", err)
		}

		dest := s.notificationTopicARN
		if dest == "" {
			dest = "notification-topic"
		}

		outbox := models.OutboxEvent{
			ID:              uuid.New(),
			AggregateType:   "order",
			AggregateID:     item.OrderID,
			EventType:       "order.price_drop_refund.requested",
			DestinationType: models.OutboxDestinationSNS,
			Destination:     dest,
			Payload:         payloadBytes,
			Status:          models.OutboxStatusPending,
			AvailableAt:     time.Now().UTC(),
		}
		outboxEvents = append(outboxEvents, outbox)
	}

	if err := s.repo.RecordRefundOutboxEvents(ctx, outboxEvents); err != nil {
		return nil, fmt.Errorf("failed to record refund outbox events: %w", err)
	}

	s.logger.Info("Successfully processed 14-day price drop refunds",
		zap.String("product_id", event.ProductID),
		zap.Int("orders_affected", len(orderItems)),
		zap.Int("outbox_events_created", len(outboxEvents)),
		zap.Float64("total_refunded", float64(totalRefundCents)/100.0),
	)

	return &PriceDropResult{
		EligibleOrdersCount: len(orderItems),
		TotalRefundAmount:   float64(totalRefundCents) / 100.0,
		OutboxEventsCreated: len(outboxEvents),
	}, nil
}
