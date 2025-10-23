package services

import (
	"context"
	"encoding/json"
	"payment-service/kafka"
	"payment-service/models"
	"time"

	kafkago "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type PaymentRequestConsumer struct {
	reader          *kafkago.Reader
	paymentProducer *kafka.PaymentEventProducer
	stripeSvc       *StripeService
	logger          *zap.Logger
}

func NewPaymentRequestConsumer(brokers []string, groupID string, producer *kafka.PaymentEventProducer, stripeSvc *StripeService) *PaymentRequestConsumer {
	r := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:  brokers,
		Topic:    "payment-requests",
		GroupID:  groupID,
		MinBytes: 1e3,
		MaxBytes: 10e6,
	})
	zap.L().Info("PaymentRequestConsumer initialized", zap.String("topic", "payment-requests"), zap.Strings("brokers", brokers), zap.String("group_id", groupID))
	return &PaymentRequestConsumer{reader: r, paymentProducer: producer, stripeSvc: stripeSvc, logger: zap.L()}
}

func (c *PaymentRequestConsumer) Start() {
	c.logger.Info("Starting PaymentRequestConsumer", zap.String("topic", "payment-requests"))

	for {
		m, err := c.reader.ReadMessage(context.Background())
		if err != nil {
			c.logger.Warn("Error reading payment request", zap.Error(err))
			continue
		}

		var req models.PaymentRequest
		if err := json.Unmarshal(m.Value, &req); err != nil {
			c.logger.Warn("Invalid payment request JSON", zap.Error(err), zap.String("payload", string(m.Value)))
			continue
		}

		// Simulate payment processing (fake delay)
		time.Sleep(2 * time.Second)
		amountForStripe := int64(req.Amount) * 100

		paymentID, err := c.stripeSvc.CreatePaymentIntent(amountForStripe, "INR")
		var eventType string
		if err != nil {
			c.logger.Warn("Stripe charge failed", zap.String("order_id", req.OrderID), zap.Error(err))
			eventType = "payment_failed"
		} else {
			c.logger.Info("Stripe charge succeeded", zap.String("order_id", req.OrderID), zap.String("payment_id", paymentID))
			eventType = "payment_succeeded"
		}

		// if req.Amount%2 == 0 { // fake rule: even amounts succeed
		// 	eventType = "payment_succeeded"
		// }
		c.logger.Info("Payment processing result", zap.String("order_id", req.OrderID), zap.String("event_type", eventType))

		event := models.PaymentEvent{
			OrderID:   req.OrderID,
			UserID:    req.UserID,
			Type:      eventType,
			Amount:    req.Amount,
			PaymentID: paymentID,
			Currency:  "INR",
			Timestamp: time.Now().UTC(),
		}

		if err := c.paymentProducer.SendPaymentEvent(event); err != nil {
			c.logger.Warn("Failed to publish payment event", zap.String("order_id", req.OrderID), zap.Error(err))
		} else {
			c.logger.Info("Payment event published successfully", zap.Any("event", event))
		}
	}
}
