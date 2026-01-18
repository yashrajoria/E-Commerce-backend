package services

import (
	"context"
	"encoding/json"
	"log"
	"order-service/models"
	"time"

	"github.com/segmentio/kafka-go"
	"gorm.io/gorm"
)

type PaymentConsumer struct {
	reader *kafka.Reader
	db     *gorm.DB
	topic  string
	group  string
}

func NewPaymentConsumer(brokers []string, topic, groupID string, db *gorm.DB) *PaymentConsumer {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  groupID,
		MinBytes: 1e3,
		MaxBytes: 10e6,
	})
	log.Printf("[OrderService][PaymentConsumer] initialized topic=%s group=%s brokers=%v", topic, groupID, brokers)
	return &PaymentConsumer{reader: r, db: db, topic: topic, group: groupID}
}

func (pc *PaymentConsumer) Start() {
	log.Printf("[OrderService][PaymentConsumer] listening topic=%s group=%s", pc.topic, pc.group)

	for {
		m, err := pc.reader.ReadMessage(context.Background())
		if err != nil {
			log.Printf("❌ [OrderService][PaymentConsumer] read error: %v", err)
			continue
		}

		var evt models.PaymentEvent
		if err := json.Unmarshal(m.Value, &evt); err != nil {
			log.Printf("❌ [OrderService][PaymentConsumer] invalid JSON: %v payload=%s", err, string(m.Value))
			continue
		}
		if evt.OrderID == "" || evt.Type == "" {
			log.Printf("❌ [OrderService][PaymentConsumer] missing fields: order_id=%q type=%q", evt.OrderID, evt.Type)
			continue
		}

		log.Printf("ℹ️  [OrderService][PaymentConsumer] received event: order_id=%s type=%s", evt.OrderID, evt.Type)

		now := time.Now()
		switch evt.Type {
		case "payment_succeeded":
			pc.updateOrderStatusWithTime(evt.OrderID, "paid", &now, nil)
		case "payment_failed":
			pc.updateOrderStatusWithTime(evt.OrderID, "payment_failed", nil, &now)
		case "checkout_session_created":
			log.Printf("ℹ️  [OrderService][PaymentConsumer] checkout session created for order=%s", evt.OrderID)
		case "checkout_session_failed":
			pc.updateOrderStatusWithTime(evt.OrderID, "payment_failed", nil, &now)
		default:
			log.Printf("⚠️  [OrderService][PaymentConsumer] unknown event type: %s", evt.Type)
		}
	}
}

func (pc *PaymentConsumer) updateOrderStatusWithTime(orderID, status string, completedAt, canceledAt *time.Time) {
	updateFields := map[string]interface{}{
		"status": status,
	}
	if completedAt != nil {
		updateFields["completed_at"] = *completedAt
	}
	if canceledAt != nil {
		updateFields["canceled_at"] = *canceledAt
	}

	err := pc.db.Transaction(func(tx *gorm.DB) error {
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
				log.Printf("ℹ️  [OrderService][PaymentConsumer] order=%s already %s; skipping", orderID, status)
				return nil
			}
		}
		return tx.Model(&order).Updates(updateFields).Error
	})
	if err != nil {
		log.Printf("❌ [OrderService][PaymentConsumer] failed to update order=%s: %v", orderID, err)
	} else {
		log.Printf("✅ [OrderService][PaymentConsumer] order=%s updated to %s", orderID, status)
	}
}

func (pc *PaymentConsumer) Close() error {
	log.Printf("[OrderService][PaymentConsumer] closing reader topic=%s group=%s", pc.topic, pc.group)
	return pc.reader.Close()
}
