package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"order-service/payment/repository"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v86"
	"gorm.io/gorm"
)

var (
	ErrPaymentNotFound        = errors.New("payment record not found")
	ErrPaymentAlreadyRefunded = errors.New("payment already refunded")
	ErrPaymentNotSucceeded    = errors.New("payment not in succeeded state")
	ErrMissingStripePaymentID = errors.New("payment has no stripe id")
)

// stripeRefundAPI is the slice of StripeService the refunder needs.
type stripeRefundAPI interface {
	PaymentIntentID(stripeID string) (string, error)
	RefundPaymentIntent(paymentIntentID, idempotencyKey string) error
	Refund(paymentIntentID string, amount int64, reason string, idempotencyKey string) (*stripe.Refund, error)
}

// RefundDetails contains information about the processed refund.
type RefundDetails struct {
	RefundID      string    `json:"refund_id"`
	Amount        int       `json:"amount"`
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	PaymentID     string    `json:"payment_id"`
	UserID        string    `json:"user_id"`
	CorrelationID string    `json:"correlation_id"`
	RefundedAt    time.Time `json:"refunded_at"`
}

// OrderRefunder fully refunds the successful Stripe payment behind an order.
type OrderRefunder struct {
	repo   repository.PaymentRepository
	stripe stripeRefundAPI
}

func NewOrderRefunder(repo repository.PaymentRepository, stripe stripeRefundAPI) *OrderRefunder {
	return &OrderRefunder{repo: repo, stripe: stripe}
}

// RefundOrder is idempotent: an already-refunded or never-charged payment is a no-op,
// and the Stripe idempotency key makes a retry after a partial failure safe.
func (r *OrderRefunder) RefundOrder(ctx context.Context, orderID uuid.UUID) error {
	payment, err := r.repo.GetPaymentByOrderID(ctx, orderID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil // nothing was charged through us
	}
	if err != nil {
		return err
	}
	if payment.Status != "succeeded" {
		return nil
	}
	if payment.StripePaymentID == nil || *payment.StripePaymentID == "" {
		return fmt.Errorf("payment for order %s has no stripe id", orderID)
	}
	piID, err := r.stripe.PaymentIntentID(*payment.StripePaymentID)
	if err != nil {
		return fmt.Errorf("resolve payment intent: %w", err)
	}
	if err := r.stripe.RefundPaymentIntent(piID, "refund-order-"+orderID.String()); err != nil {
		return fmt.Errorf("stripe refund: %w", err)
	}
	now := time.Now().UTC()
	return r.repo.Update(ctx, orderID, map[string]interface{}{
		"status":     "refunded",
		"updated_at": now,
	})
}

// RefundPayment refunds a payment with optional partial amount and reason, returning refund details.
// Unlike RefundOrder (which no-ops for cancellations of unpaid orders), RefundPayment expects a valid
// succeeded payment record and returns explicit errors if missing, already refunded, or in an invalid state.
func (r *OrderRefunder) RefundPayment(ctx context.Context, orderID uuid.UUID, amount *int, reason string, idempotencyKey string) (*RefundDetails, error) {
	payment, err := r.repo.GetPaymentByOrderID(ctx, orderID)
	if errors.Is(err, gorm.ErrRecordNotFound) || payment == nil {
		return nil, ErrPaymentNotFound
	}
	if err != nil {
		return nil, err
	}
	if payment.Status == "refunded" {
		return nil, ErrPaymentAlreadyRefunded
	}
	if payment.Status != "succeeded" {
		return nil, fmt.Errorf("%w: current status %s", ErrPaymentNotSucceeded, payment.Status)
	}
	if payment.StripePaymentID == nil || *payment.StripePaymentID == "" {
		return nil, fmt.Errorf("%w for order %s", ErrMissingStripePaymentID, orderID)
	}

	piID, err := r.stripe.PaymentIntentID(*payment.StripePaymentID)
	if err != nil {
		return nil, fmt.Errorf("resolve payment intent: %w", err)
	}

	refundAmount := int64(payment.Amount)
	if amount != nil && *amount > 0 {
		refundAmount = int64(*amount)
	}

	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("refund-order-%s-%d", orderID.String(), refundAmount)
	}

	stRefund, err := r.stripe.Refund(piID, refundAmount, reason, idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("stripe refund: %w", err)
	}

	now := time.Now().UTC()
	refundID := ""
	refundStatus := "succeeded"
	currency := payment.Currency
	if stRefund != nil {
		refundID = stRefund.ID
		if string(stRefund.Status) != "" {
			refundStatus = string(stRefund.Status)
		}
		if string(stRefund.Currency) != "" {
			currency = string(stRefund.Currency)
		}
	}

	payloadBytes, _ := json.Marshal(map[string]interface{}{
		"refund_id":         refundID,
		"amount":            refundAmount,
		"currency":          currency,
		"status":            refundStatus,
		"reason":            reason,
		"payment_intent_id": piID,
		"refunded_at":       now,
	})
	payloadStr := string(payloadBytes)

	updates := map[string]interface{}{
		"status":               "refunded",
		"stripe_event_payload": payloadStr,
		"updated_at":           now,
	}

	if err := r.repo.Update(ctx, orderID, updates); err != nil {
		return nil, fmt.Errorf("update payment status: %w", err)
	}

	return &RefundDetails{
		RefundID:      refundID,
		Amount:        int(refundAmount),
		Currency:      currency,
		Status:        refundStatus,
		PaymentID:     payment.Payment_ID.String(),
		UserID:        payment.UserID.String(),
		CorrelationID: payment.CorrelationID,
		RefundedAt:    now,
	}, nil
}
