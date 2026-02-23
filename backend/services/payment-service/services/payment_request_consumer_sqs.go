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

		// Create Stripe Checkout Session (provides a hosted URL for the user to complete payment)
		sess, err := c.stripeSvc.CreateCheckoutSession(int64(req.Amount*100), "usd", req.OrderID, req.UserID)
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
