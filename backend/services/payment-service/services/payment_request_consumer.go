package services

import (
	"context"
	"encoding/json"
	"payment-service/kafka"
	"payment-service/models"
	"payment-service/repository"
	"strings"
	"time"

	"github.com/google/uuid"
	kafkago "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type PaymentRequestConsumer struct {
	reader          *kafkago.Reader
	paymentProducer *kafka.PaymentEventProducer
	stripeSvc       *StripeService
	logger          *zap.Logger
	repo            repository.PaymentRepository
	topic           string
}

func NewPaymentRequestConsumer(brokers []string, topic, groupID string, producer *kafka.PaymentEventProducer, stripeSvc *StripeService, repo repository.PaymentRepository, logger *zap.Logger) *PaymentRequestConsumer {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		zap.L().Fatal("PaymentRequestConsumer topic is empty")
	}
	r := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  groupID,
		MinBytes: 1e3,
		MaxBytes: 10e6,
	})
	zap.L().Info("PaymentRequestConsumer initialized", zap.String("topic", topic), zap.Strings("brokers", brokers), zap.String("group_id", groupID))
	return &PaymentRequestConsumer{reader: r, paymentProducer: producer, stripeSvc: stripeSvc, logger: zap.L(), repo: repo, topic: topic}
}

func (c *PaymentRequestConsumer) Start() {
	c.logger.Info("Starting PaymentRequestConsumer", zap.String("topic", c.topic))
	ctx := context.Background() // Use this context for DB calls
	for {
		m, err := c.reader.ReadMessage(ctx)
		if err != nil {
			c.logger.Warn("Error reading payment request", zap.Error(err))
			continue
		}

		var req models.PaymentRequest
		if err := json.Unmarshal(m.Value, &req); err != nil {
			c.logger.Warn("Invalid payment request JSON", zap.Error(err), zap.String("payload", string(m.Value)))
			continue
		}

		orderID, err := uuid.Parse(req.OrderID)
		if err != nil {
			c.logger.Warn("Invalid OrderID", zap.String("order_id", req.OrderID), zap.Error(err))
			continue
		}
		userID, err := uuid.Parse(req.UserID)
		if err != nil {
			c.logger.Warn("Invalid UserID", zap.String("user_id", req.UserID), zap.Error(err))
			continue
		}
		amountForStripe := int64(req.Amount) * 100

		payment := &models.Payment{
			Payment_ID: uuid.New(),
			OrderID:    orderID,
			UserID:     userID,
			Amount:     int(amountForStripe),
			Currency:   "inr",
			Status:     "PROCESSING",
		}
		if err := c.repo.CreatePayment(ctx, payment); err != nil {
			c.logger.Error("Failed to create initial payment record", zap.String("order_id", req.OrderID), zap.Error(err))
			continue // Retry message
		}
		// Simulate payment processing (fake delay)
		time.Sleep(2 * time.Second)

		session, err := c.stripeSvc.CreateCheckoutSession(amountForStripe, "inr", req.OrderID, req.UserID)
		var eventType string
		var urlForDB *string
		checkoutURL := ""

		if err != nil {
			c.logger.Warn("Stripe checkout session creation failed", zap.String("order_id", req.OrderID), zap.Error(err))
			eventType = "checkout_session_failed"
			// --- 3. UPDATE DB ON FAILURE ---
			if err_db := c.repo.UpdatePaymentByOrderID(ctx, orderID, "FAILED", nil, nil); err_db != nil {
				c.logger.Error("Failed to update payment status to FAILED", zap.String("order_id", req.OrderID), zap.Error(err_db))
			}
			// ---
		} else {
			c.logger.Info("Stripe checkout session created successfully", zap.String("order_id", req.OrderID), zap.String("session_url", session.URL))
			eventType = "checkout_session_created"
			urlForDB = &session.URL
			checkoutURL = session.URL
			// --- 4. UPDATE DB ON SUCCESS ---
			// Pass session.ID to be saved as stripe_payment_id
			if err_db := c.repo.UpdatePaymentByOrderID(ctx, orderID, "URL_READY", urlForDB, &session.ID); err_db != nil {
				c.logger.Error("Failed to update payment status to URL_READY", zap.String("order_id", req.OrderID), zap.Error(err_db))
			}
			// ---
		}

		c.logger.Info("Payment processing result", zap.String("order_id", req.OrderID), zap.String("event_type", eventType))

		// --- 5. SEND KAFKA EVENT (Your existing logic is fine) ---
		event := models.PaymentEvent{
			OrderID:     req.OrderID,
			UserID:      req.UserID,
			Type:        eventType,
			Amount:      req.Amount,
			Currency:    "INR",
			CheckoutURL: checkoutURL,
			Timestamp:   time.Now().UTC(),
		}

		if err := c.paymentProducer.SendPaymentEvent(event); err != nil {
			c.logger.Warn("Failed to publish payment event", zap.String("order_id", req.OrderID), zap.Error(err))
		} else {
			c.logger.Info("Payment event published successfully", zap.Any("event", event))
		}
	}
}
