package services

import (
	"context"
	"encoding/json"
	"log"
	"payment-service/kafka"
	"payment-service/models"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

type PaymentRequestConsumer struct {
	reader          *kafkago.Reader
	paymentProducer *kafka.PaymentEventProducer
}

func NewPaymentRequestConsumer(brokers []string, groupID string, producer *kafka.PaymentEventProducer) *PaymentRequestConsumer {
	r := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:  brokers,
		Topic:    "payment-requests",
		GroupID:  groupID,
		MinBytes: 1e3,
		MaxBytes: 10e6,
	})
	log.Printf("[PaymentService] 🔌 PaymentRequestConsumer initialized. Brokers=%v, Topic=payment-requests, GroupID=%s", brokers, groupID)
	return &PaymentRequestConsumer{reader: r, paymentProducer: producer}
}

func (c *PaymentRequestConsumer) Start() {
	log.Println("[PaymentService] 🚀 Listening for payment-requests...")

	for {
		m, err := c.reader.ReadMessage(context.Background())
		if err != nil {
			log.Printf("[PaymentService] ❌ Error reading payment request: %v", err)
			continue
		}
		log.Printf("[PaymentService][Kafka] Received message at offset %d", m.Offset)

		var req models.PaymentRequest
		if err := json.Unmarshal(m.Value, &req); err != nil {
			log.Printf("[PaymentService] ⚠️ Invalid payment request JSON: %v Payload=%s", err, string(m.Value))
			continue
		}

		log.Printf("[PaymentService] 🛒 Received PaymentRequest OrderID=%s Amount=%d Currency=%s",
			req.OrderID, req.Amount, req.Currency)

		// Simulate payment processing (fake delay)
		time.Sleep(2 * time.Second)

		eventType := "payment_failed"
		if req.Amount%2 == 0 { // fake rule: even amounts succeed
			eventType = "payment_succeeded"
		}
		log.Printf("[PaymentService] 🎯 Payment processing result for Order=%s: %s", req.OrderID, eventType)

		event := models.PaymentEvent{
			OrderID:   req.OrderID,
			UserID:    req.UserID,
			Type:      eventType,
			Amount:    req.Amount,
			Currency:  req.Currency,
			Timestamp: time.Now().UTC(),
		}

		if err := c.paymentProducer.SendPaymentEvent(event); err != nil {
			log.Printf("[PaymentService] ❌ Failed to publish payment event for Order=%s: %v", req.OrderID, err)
		} else {
			log.Printf("[PaymentService] ✅ Payment event published successfully: %+v", event)
		}
	}
}
