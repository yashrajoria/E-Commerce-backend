package services

import (
	"context"
	"encoding/json"
	"payment-service/models"
	"payment-service/repository"
	"time"

	"github.com/google/uuid"
	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"go.uber.org/zap"
)

type PaymentRequestConsumer struct {
	sqsConsumer     *aws_pkg.SQSConsumer
	snsPublisher    *aws_pkg.SNSClient
	paymentTopicArn string
	stripeSvc       *StripeService
	logger          *zap.Logger
	repo            repository.PaymentRepository
}

func NewPaymentRequestConsumer(
	sqsConsumer *aws_pkg.SQSConsumer,
	snsPublisher *aws_pkg.SNSClient,
	paymentTopicArn string,
	stripeSvc *StripeService,
	repo repository.PaymentRepository,
	logger *zap.Logger,
) *PaymentRequestConsumer {
	return &PaymentRequestConsumer{
		sqsConsumer:     sqsConsumer,
		snsPublisher:    snsPublisher,
		paymentTopicArn: paymentTopicArn,
		stripeSvc:       stripeSvc,
		logger:          logger,
		repo:            repo,
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

		// Create payment record
		payment := models.Payment{
			Payment_ID: uuid.New(),
			OrderID:    orderID,
			UserID:     userID,
			Amount:     req.Amount,
			Currency:   "usd",
			Status:     "pending",
			CreatedAt:  time.Now().UTC(),
		}

		if err := c.repo.CreatePayment(ctx, &payment); err != nil {
			c.logger.Error("Failed to create payment record", zap.Error(err))
			return err
		}

		c.logger.Info("Payment record created", zap.String("payment_id", payment.Payment_ID.String()))

		// Create Stripe PaymentIntent
		pi, err := c.stripeSvc.CreatePaymentIntent(int64(req.Amount*100), "usd")
		if err != nil {
			c.logger.Error("Failed to create Stripe PaymentIntent", zap.Error(err))
			payment.Status = "failed"
			c.repo.CreatePayment(ctx, &payment)

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
			return err
		}

		payment.StripePaymentID = &pi.ID
		// Note: Payment model doesn't have ClientSecret field
		if err := c.repo.CreatePayment(ctx, &payment); err != nil {
			c.logger.Warn("Failed to save payment with Stripe ID", zap.Error(err))
		}

		c.logger.Info("Payment request processed",
			zap.String("order_id", req.OrderID),
			zap.String("payment_id", payment.Payment_ID.String()),
		)

		return nil
	})

	if err != nil && err != context.Canceled {
		c.logger.Error("SQS consumer error", zap.Error(err))
	}
}
