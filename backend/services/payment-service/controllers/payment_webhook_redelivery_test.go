package controllers

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"

	"payment-service/models"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v80"
	"go.uber.org/zap"
)

// mockPaymentRepo mimics the Postgres row semantics that matter for this test:
// UpdateIfStatusNotIn only succeeds for one caller once the status becomes terminal.
type mockPaymentRepo struct {
	mu      sync.Mutex
	payment models.Payment
}

func (m *mockPaymentRepo) CreatePayment(context.Context, *models.Payment) error { return nil }

func (m *mockPaymentRepo) GetPaymentByOrderID(_ context.Context, orderID uuid.UUID) (*models.Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.payment
	return &p, nil
}

func (m *mockPaymentRepo) GetPaymentByIdempotencyKey(context.Context, string) (*models.Payment, error) {
	return nil, nil
}

func (m *mockPaymentRepo) GetPaymentByStripeID(context.Context, string) (*models.Payment, error) {
	return nil, nil
}

func (m *mockPaymentRepo) UpdatePaymentByOrderID(context.Context, uuid.UUID, string, *string, *string) error {
	return nil
}

func (m *mockPaymentRepo) Update(_ context.Context, _ uuid.UUID, updates map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if status, ok := updates["status"].(string); ok {
		m.payment.Status = status
	}
	return nil
}

func (m *mockPaymentRepo) UpdateIfStatusNotIn(_ context.Context, _ uuid.UUID, exclude []string, updates map[string]interface{}) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range exclude {
		if m.payment.Status == s {
			return false, nil
		}
	}
	if status, ok := updates["status"].(string); ok {
		m.payment.Status = status
	}
	return true, nil
}

func (m *mockPaymentRepo) MarkStripeEventProcessed(context.Context, string, string) (bool, error) {
	return true, nil
}

type countingSNSPublisher struct {
	calls int32
}

func (c *countingSNSPublisher) Publish(context.Context, string, []byte) error {
	atomic.AddInt32(&c.calls, 1)
	return nil
}

// TestStripeWebhook_ConcurrentRedelivery proves that two near-simultaneous deliveries of the
// same checkout.session.completed webhook (Stripe's documented at-least-once behavior) result
// in exactly one payment_succeeded publish, not two — the conditional UPDATE, not the
// pre-read status check, is what must serialize the race.
func TestStripeWebhook_ConcurrentRedelivery(t *testing.T) {
	orderID := uuid.New()
	userID := uuid.New()

	repo := &mockPaymentRepo{payment: models.Payment{
		Payment_ID: uuid.New(),
		OrderID:    orderID,
		UserID:     userID,
		Amount:     4999,
		Currency:   "usd",
		Status:     "pending",
	}}
	sns := &countingSNSPublisher{}

	pc := &PaymentController{
		Repo:     repo,
		SNS:      sns,
		TopicArn: "payment-events",
		Logger:   zap.NewNop(),
	}

	session := stripe.CheckoutSession{
		ID: "cs_test_123",
		Metadata: map[string]string{
			"order_id": orderID.String(),
			"user_id":  userID.String(),
		},
	}
	raw, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	event := stripe.Event{
		ID:   "evt_redelivered_1",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{Raw: raw},
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = pc.handleCheckoutCompleted(event, raw)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("delivery %d returned error: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&sns.calls); got != 1 {
		t.Fatalf("expected exactly 1 payment_succeeded publish for duplicate webhook delivery, got %d", got)
	}
	if repo.payment.Status != "succeeded" {
		t.Fatalf("expected payment status succeeded, got %q", repo.payment.Status)
	}
}
