package services

import (
	"context"
	"errors"
	"fmt"

	"order-service/payment/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// stripeRefundAPI is the slice of StripeService the refunder needs.
type stripeRefundAPI interface {
	PaymentIntentID(stripeID string) (string, error)
	RefundPaymentIntent(paymentIntentID, idempotencyKey string) error
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
	return r.repo.Update(ctx, orderID, map[string]interface{}{"status": "refunded"})
}
