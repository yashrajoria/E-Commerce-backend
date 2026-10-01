package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"order-service/models"
	repositories "order-service/repository"

	"github.com/google/uuid"
)

// duplicateCheckoutRepo is a stub OrderRepository that tracks CreateWithOutbox
// calls and returns a pre-loaded order for FindByIdempotencyKey on the second call.
type duplicateCheckoutRepo struct {
	repositories.OrderRepository

	createCallCount int
	storedOrder     *models.Order
}

func (r *duplicateCheckoutRepo) CreateWithOutbox(_ context.Context, order *models.Order, _ []models.OutboxEvent) error {
	r.createCallCount++
	if r.createCallCount == 1 {
		// First call succeeds; persist the order.
		copy := *order
		r.storedOrder = &copy
		return nil
	}
	// Simulate a unique constraint violation on the idempotency key on the second call.
	return errDuplicateIdempotencyKey
}

func (r *duplicateCheckoutRepo) FindByIdempotencyKey(_ context.Context, key string) (*models.Order, error) {
	if r.storedOrder != nil {
		if r.storedOrder.IdempotencyKey != nil && *r.storedOrder.IdempotencyKey == key {
			copy := *r.storedOrder
			return &copy, nil
		}
	}
	return nil, nil
}

// errDuplicateIdempotencyKey simulates the DB error returned when the
// idempotency key UNIQUE index fires (GORM returns the underlying DB error).
var errDuplicateIdempotencyKey = &duplicateKeyError{}

type duplicateKeyError struct{}

func (e *duplicateKeyError) Error() string { return "duplicate key value violates unique constraint" }

// TestDuplicateCheckoutRequest_SecondCallIsIdempotent verifies that when the
// same checkout message is delivered twice (at-least-once SQS delivery), the
// second delivery is silently skipped without creating a second order.
//
// This guards the path: CreateWithOutbox fails with a unique-key violation →
// FindByIdempotencyKey finds the existing order → handler returns nil (ack).
func TestDuplicateCheckoutRequest_SecondCallIsIdempotent(t *testing.T) {
	productID := uuid.New()
	orderID := uuid.New()
	const idemKey = "user-abc:checkout-xyz-hash"

	// Product server: always returns a fixed product.
	productServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"` + productID.String() + `","name":"Test Widget","price":29.99}`))
	}))
	defer productServer.Close()

	// Inventory server: always succeeds for reserve.
	inventoryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer inventoryServer.Close()

	repo := &duplicateCheckoutRepo{}
	consumer := NewSQSCheckoutConsumer(
		nil, nil, repo,
		NewInventoryClient(inventoryServer.URL),
		nil,
		productServer.URL,
		nil, "", nil, "usd",
	)

	msgBody := `{
		"event":"checkout.requested",
		"user_id":"` + uuid.NewString() + `",
		"order_id":"` + orderID.String() + `",
		"idempotency_key":"` + idemKey + `",
		"correlation_id":"corr-dup-1",
		"items":[{"product_id":"` + productID.String() + `","quantity":2}]
	}`

	// First delivery — must succeed and create the order.
	if err := consumer.handleMessage(context.Background(), msgBody); err != nil {
		t.Fatalf("first handleMessage: unexpected error: %v", err)
	}
	if repo.createCallCount != 1 {
		t.Fatalf("expected 1 CreateWithOutbox call after first delivery, got %d", repo.createCallCount)
	}

	// Manually set the idempotency key on the stored order so FindByIdempotencyKey works.
	idemKeyCopy := idemKey
	repo.storedOrder.IdempotencyKey = &idemKeyCopy

	// Second delivery of the same message (SQS at-least-once) — must be silently
	// skipped (return nil = ack) without creating a second order.
	if err := consumer.handleMessage(context.Background(), msgBody); err != nil {
		t.Fatalf("second handleMessage (duplicate): unexpected error: %v", err)
	}
	// CreateWithOutbox was called a second time (duplicate key), but the handler
	// recovered via FindByIdempotencyKey and returned nil.
	if repo.createCallCount != 2 {
		t.Fatalf("expected 2 CreateWithOutbox calls total (1 real + 1 duplicate), got %d", repo.createCallCount)
	}
}
