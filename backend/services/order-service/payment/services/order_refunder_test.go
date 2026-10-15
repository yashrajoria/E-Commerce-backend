package services

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"order-service/payment/models"
	"order-service/payment/repository"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v86"
	"gorm.io/gorm"
)

type refundRepo struct {
	repository.PaymentRepository
	payment models.Payment
	updates map[string]interface{}
}

func (r *refundRepo) GetPaymentByOrderID(context.Context, uuid.UUID) (*models.Payment, error) {
	p := r.payment
	return &p, nil
}
func (r *refundRepo) Update(_ context.Context, _ uuid.UUID, u map[string]interface{}) error {
	r.updates = u
	return nil
}

type fakeStripeRefund struct {
	refunded []string
	failErr  error
}

func (f *fakeStripeRefund) PaymentIntentID(id string) (string, error) { return "pi_" + id, nil }
func (f *fakeStripeRefund) RefundPaymentIntent(pi, key string) error {
	if f.failErr != nil {
		return f.failErr
	}
	f.refunded = append(f.refunded, pi+"|"+key)
	return nil
}
func (f *fakeStripeRefund) Refund(pi string, amount int64, reason string, key string) (*stripe.Refund, error) {
	if f.failErr != nil {
		return nil, f.failErr
	}
	f.refunded = append(f.refunded, fmt.Sprintf("%s|%d|%s|%s", pi, amount, reason, key))
	return &stripe.Refund{
		ID:       "re_test_123",
		Amount:   amount,
		Currency: "usd",
		Status:   stripe.RefundStatusSucceeded,
	}, nil
}

func TestOrderRefunder_RefundsSucceededOnlyOnce(t *testing.T) {
	id := "cs_test_1"
	orderID := uuid.New()
	repo := &refundRepo{payment: models.Payment{Status: "succeeded", StripePaymentID: &id}}
	st := &fakeStripeRefund{}
	r := NewOrderRefunder(repo, st)

	if err := r.RefundOrder(context.Background(), orderID); err != nil {
		t.Fatal(err)
	}
	if len(st.refunded) != 1 || st.refunded[0] != "pi_cs_test_1|refund-order-"+orderID.String() {
		t.Fatalf("unexpected refunds: %v", st.refunded)
	}
	if repo.updates["status"] != "refunded" {
		t.Fatalf("payment not marked refunded: %v", repo.updates)
	}

	// Already refunded / never charged -> no Stripe call.
	for _, status := range []string{"refunded", "pending", "failed"} {
		repo.payment.Status = status
		if err := r.RefundOrder(context.Background(), orderID); err != nil {
			t.Fatal(err)
		}
	}
	if len(st.refunded) != 1 {
		t.Fatalf("expected no further refunds, got %v", st.refunded)
	}
}

func TestOrderRefunder_RefundPayment_Success(t *testing.T) {
	id := "pi_test_abc"
	orderID := uuid.New()
	userID := uuid.New()
	paymentID := uuid.New()
	repo := &refundRepo{
		payment: models.Payment{
			Payment_ID:      paymentID,
			OrderID:         orderID,
			UserID:          userID,
			Status:          "succeeded",
			Amount:          5000,
			Currency:        "usd",
			StripePaymentID: &id,
			CorrelationID:   "corr-123",
		},
	}
	st := &fakeStripeRefund{}
	r := NewOrderRefunder(repo, st)

	partialAmount := 2500
	details, err := r.RefundPayment(context.Background(), orderID, &partialAmount, "requested_by_customer", "idem-123")
	if err != nil {
		t.Fatalf("RefundPayment failed: %v", err)
	}

	if details.RefundID != "re_test_123" {
		t.Fatalf("expected refund ID re_test_123, got %s", details.RefundID)
	}
	if details.Amount != 2500 {
		t.Fatalf("expected amount 2500, got %d", details.Amount)
	}
	if repo.updates["status"] != "refunded" {
		t.Fatalf("expected status refunded in updates, got %v", repo.updates["status"])
	}
	if repo.updates["stripe_event_payload"] == nil {
		t.Fatal("expected stripe_event_payload to be recorded")
	}
	if len(st.refunded) != 1 || st.refunded[0] != "pi_pi_test_abc|2500|requested_by_customer|idem-123" {
		t.Fatalf("unexpected stripe call: %v", st.refunded)
	}
}

type missingPaymentRepo struct {
	repository.PaymentRepository
}

func (m *missingPaymentRepo) GetPaymentByOrderID(context.Context, uuid.UUID) (*models.Payment, error) {
	return nil, gorm.ErrRecordNotFound
}

func TestOrderRefunder_RefundPayment_Errors(t *testing.T) {
	orderID := uuid.New()

	// 1. Missing payment
	rMissing := NewOrderRefunder(&missingPaymentRepo{}, &fakeStripeRefund{})
	_, err := rMissing.RefundPayment(context.Background(), orderID, nil, "", "")
	if !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("expected ErrPaymentNotFound, got %v", err)
	}

	// 2. Already refunded payment
	id := "pi_123"
	repo := &refundRepo{payment: models.Payment{Status: "refunded", StripePaymentID: &id}}
	r := NewOrderRefunder(repo, &fakeStripeRefund{})
	_, err = r.RefundPayment(context.Background(), orderID, nil, "", "")
	if !errors.Is(err, ErrPaymentAlreadyRefunded) {
		t.Fatalf("expected ErrPaymentAlreadyRefunded, got %v", err)
	}

	// 3. Payment not in succeeded status (e.g. pending)
	repo.payment.Status = "pending"
	_, err = r.RefundPayment(context.Background(), orderID, nil, "", "")
	if !errors.Is(err, ErrPaymentNotSucceeded) {
		t.Fatalf("expected ErrPaymentNotSucceeded, got %v", err)
	}

	// 4. Missing stripe payment ID
	repo.payment.Status = "succeeded"
	repo.payment.StripePaymentID = nil
	_, err = r.RefundPayment(context.Background(), orderID, nil, "", "")
	if !errors.Is(err, ErrMissingStripePaymentID) {
		t.Fatalf("expected ErrMissingStripePaymentID, got %v", err)
	}

	// 5. Stripe error
	repo.payment.StripePaymentID = &id
	stErr := &fakeStripeRefund{failErr: errors.New("stripe card declined / API error")}
	rFail := NewOrderRefunder(repo, stErr)
	_, err = rFail.RefundPayment(context.Background(), orderID, nil, "", "")
	if err == nil {
		t.Fatal("expected Stripe failure error, got nil")
	}
}
