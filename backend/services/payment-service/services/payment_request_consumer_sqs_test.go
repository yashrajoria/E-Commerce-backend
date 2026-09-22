package services

import (
	"context"
	"errors"
	"testing"

	"payment-service/models"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v80"
	"go.uber.org/zap"
)

type duplicatePaymentRepo struct {
	claimErr error
}

func (duplicatePaymentRepo) CreatePayment(context.Context, *models.Payment) error { return nil }
func (duplicatePaymentRepo) GetPaymentByOrderID(context.Context, uuid.UUID) (*models.Payment, error) {
	return nil, nil
}
func (duplicatePaymentRepo) GetPaymentByIdempotencyKey(context.Context, string) (*models.Payment, error) {
	return nil, nil
}
func (duplicatePaymentRepo) GetPaymentByStripeID(context.Context, string) (*models.Payment, error) {
	return nil, nil
}
func (duplicatePaymentRepo) UpdatePaymentByOrderID(context.Context, uuid.UUID, string, *string, *string) error {
	return nil
}
func (duplicatePaymentRepo) Update(context.Context, uuid.UUID, map[string]interface{}) error {
	return nil
}
func (duplicatePaymentRepo) UpdateIfStatusNotIn(context.Context, uuid.UUID, []string, map[string]interface{}) (bool, error) {
	return false, nil
}
func (duplicatePaymentRepo) MarkStripeEventProcessed(context.Context, string, string) (bool, error) {
	return false, nil
}
func (r duplicatePaymentRepo) ClaimPaymentRequest(context.Context, *models.Payment) (bool, error) {
	if r.claimErr != nil {
		return false, r.claimErr
	}
	return false, nil
}

type unexpectedStripeCall struct{}

func (unexpectedStripeCall) CreateCheckoutSession(int64, string, string, string) (*stripe.CheckoutSession, error) {
	panic("duplicate payment request called Stripe")
}

func TestPaymentRequestDuplicateSkipsStripe(t *testing.T) {
	consumer := NewPaymentRequestConsumer(nil, nil, "", "", unexpectedStripeCall{}, "usd", duplicatePaymentRepo{}, zap.NewNop())
	body := `{"event_id":"event-1","order_id":"` + uuid.NewString() + `","user_id":"` + uuid.NewString() + `","amount":1000,"currency":"usd","idempotency_key":"idem-1"}`

	if err := consumer.handleMessage(context.Background(), body); err != nil {
		t.Fatalf("duplicate payment request returned error: %v", err)
	}
}

func TestPaymentRequestClaimErrorStopsProcessing(t *testing.T) {
	consumer := NewPaymentRequestConsumer(nil, nil, "", "", unexpectedStripeCall{}, "usd", duplicatePaymentRepo{claimErr: errors.New("database unavailable")}, zap.NewNop())
	body := `{"event_id":"event-1","order_id":"` + uuid.NewString() + `","user_id":"` + uuid.NewString() + `","amount":1000,"currency":"usd","idempotency_key":"idem-1"}`

	if err := consumer.handleMessage(context.Background(), body); err == nil {
		t.Fatal("expected payment request claim error")
	}
}
