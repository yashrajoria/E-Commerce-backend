package services

import (
	"context"
	"encoding/json"
	"fmt"
	"payment-service/models"
	"payment-service/repository"
	"strings"
	"time"

	"github.com/google/uuid"
	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"github.com/yashrajoria/common/events"
	"go.uber.org/zap"
)

type PaymentRequestConsumer struct {
	sqsConsumer          *aws_pkg.SQSConsumer
	snsPublisher         *aws_pkg.SNSClient
	paymentTopicArn      string
	notificationTopicArn string
	stripeSvc            *StripeService
	defaultCurrency      string
	logger               *zap.Logger
	repo                 repository.PaymentRepository
}

func NewPaymentRequestConsumer(
	sqsConsumer *aws_pkg.SQSConsumer,
	snsPublisher *aws_pkg.SNSClient,
	paymentTopicArn string,
	notificationTopicArn string,
	stripeSvc *StripeService,
	defaultCurrency string,
	repo repository.PaymentRepository,
	logger *zap.Logger,
) *PaymentRequestConsumer {
	return &PaymentRequestConsumer{
		sqsConsumer:          sqsConsumer,
		snsPublisher:         snsPublisher,
		paymentTopicArn:      paymentTopicArn,
		notificationTopicArn: notificationTopicArn,
		stripeSvc:            stripeSvc,
		defaultCurrency:      normalizeCurrency(defaultCurrency),
		logger:               logger,
		repo:                 repo,
	}
}

func (c *PaymentRequestConsumer) Start(ctx context.Context) {
	c.logger.Info("Starting PaymentRequestConsumer (SQS)")

	err := c.sqsConsumer.StartPolling(ctx, func(ctx context.Context, body string) error {
		var req models.PaymentRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			c.logger.Warn("Invalid payment request JSON", zap.Error(err))
			return err
		}

		// Validate Idempotency Key
		if req.IdempotencyKey == "" {
			c.logger.Warn("Missing Idempotency-Key header")
			return fmt.Errorf("missing Idempotency-Key header")
		}

		// Check if payment already exists for the idempotency key
		if existing, err := c.repo.GetPaymentByIdempotencyKey(ctx, req.IdempotencyKey); err == nil && existing != nil {
			c.logger.Info("Payment already exists for idempotency key, skipping", zap.String("idempotency_key", req.IdempotencyKey), zap.String("payment_id", existing.Payment_ID.String()))
			return nil
		}

		orderID, err := uuid.Parse(req.OrderID)
		if err != nil {
			c.logger.Warn("Invalid order_id format", zap.String("order_id", req.OrderID), zap.Error(err))
			return err
		}

		userID, err := uuid.Parse(req.UserID)
		if err != nil {
			c.logger.Warn("Invalid user_id format", zap.String("user_id", req.UserID), zap.Error(err))
			return err
		}

		currency := normalizeCurrency(req.Currency)
		if currency == "" {
			currency = c.defaultCurrency
		}

		// Create payment record
		payment := models.Payment{
			Payment_ID: uuid.New(),
			OrderID:    orderID,
			UserID:     userID,
			Amount:     req.Amount,
			Currency:   currency,
			Status:     "pending",
			CreatedAt:  time.Now().UTC(),
		}
		if req.IdempotencyKey != "" {
			payment.IdempotencyKey = &req.IdempotencyKey
		}

		if err := c.repo.CreatePayment(ctx, &payment); err != nil {
			c.logger.Error("Failed to create payment record", zap.Error(err))
			return err
		}

		c.logger.Info("Payment record created", zap.String("payment_id", payment.Payment_ID.String()))

		// Create Stripe Checkout Session (provides a hosted URL for the user to complete payment)
		// Amount is already in the smallest currency unit for the configured store currency.
		// Do NOT multiply by 100 here; prices are stored and passed in cents throughout the system.
		sess, err := c.stripeSvc.CreateCheckoutSession(int64(req.Amount), currency, req.OrderID, req.UserID)
		if err != nil {
			c.logger.Error("Failed to create Stripe Checkout Session", zap.Error(err))
			payment.Status = "failed"
			// Update the existing payment record instead of attempting to create it again
			if updateErr := c.repo.UpdatePaymentByOrderID(ctx, orderID, "failed", nil, nil); updateErr != nil {
				c.logger.Warn("Failed to mark payment as failed", zap.Error(updateErr))
			}

			// Publish failure event
			eventMsg := models.PaymentEvent{
				Type:      "payment_failed",
				OrderID:   orderID.String(),
				UserID:    userID.String(),
				PaymentID: payment.Payment_ID.String(),
				Amount:    payment.Amount,
				Currency:  payment.Currency,
				Timestamp: time.Now().UTC(),
			}
			eventBytes, _ := json.Marshal(eventMsg)
			c.snsPublisher.Publish(ctx, c.paymentTopicArn, eventBytes)

			notificationEvent := events.NewPaymentFailedEvent(
				userID.String(),
				"",
				"",
				"",
				orderID.String(),
				float64(payment.Amount),
			)
			notificationBytes, nerr := json.Marshal(notificationEvent)
			if nerr != nil {
				c.logger.Warn("Failed to marshal payment_failed notification event", zap.Error(nerr))
			} else if perr := c.snsPublisher.Publish(ctx, c.notificationTopicArn, notificationBytes); perr != nil {
				c.logger.Warn("Failed to publish payment_failed notification event", zap.Error(perr))
			}
			return err
		}

		checkoutURL := sess.URL
		payment.StripePaymentID = &sess.ID
		// Update existing payment record with Stripe session ID and checkout URL
		if err := c.repo.UpdatePaymentByOrderID(ctx, orderID, "pending", &checkoutURL, &sess.ID); err != nil {
			c.logger.Warn("Failed to save payment with Stripe session ID", zap.Error(err))
		}

		c.logger.Info("Payment request processed",
			zap.String("order_id", req.OrderID),
			zap.String("payment_id", payment.Payment_ID.String()),
			zap.String("checkout_url", checkoutURL),
		)

		return nil
	})

	if err != nil && err != context.Canceled {
		c.logger.Error("SQS consumer error", zap.Error(err))
	}
}

func normalizeCurrency(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
