package services

import (
	"context"
	"testing"

	"order-service/payment/models"
	"order-service/payment/repository"

	"github.com/google/uuid"
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

type fakeStripeRefund struct{ refunded []string }

func (f *fakeStripeRefund) PaymentIntentID(id string) (string, error) { return "pi_" + id, nil }
func (f *fakeStripeRefund) RefundPaymentIntent(pi, key string) error {
	f.refunded = append(f.refunded, pi+"|"+key)
	return nil
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
