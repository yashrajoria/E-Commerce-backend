package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"order-service/models"
	repositories "order-service/repository"

	"github.com/google/uuid"
)

type countingSNSPublisher struct {
	calls int32
}

func (p *countingSNSPublisher) Publish(ctx context.Context, topicArn string, message []byte) error {
	atomic.AddInt32(&p.calls, 1)
	return nil
}

// fakeOrderRepo is a minimal in-memory OrderRepository for exercising the
// optimistic-locking status transition used by SQSPaymentConsumer.
type fakeOrderRepo struct {
	repositories.OrderRepository

	mu    sync.Mutex
	order models.Order
}

func (r *fakeOrderRepo) UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, fromStatus, toStatus string, extra map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.order.Status != fromStatus {
		return repositories.ErrStatusConflict
	}
	r.order.Status = toStatus
	return nil
}

func (r *fakeOrderRepo) FindByID(ctx context.Context, orderID uuid.UUID) (*models.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := r.order
	return &o, nil
}

// TestSQSPaymentConsumer_DuplicateDeliverySkipsSideEffects verifies that a
// redelivered payment_succeeded event, which loses the atomic status
// transition race, does not re-confirm inventory or re-publish the
// order_confirmed notification.
func TestSQSPaymentConsumer_DuplicateDeliverySkipsSideEffects(t *testing.T) {
	orderID := uuid.New()
	itemID := uuid.New()

	repo := &fakeOrderRepo{
		order: models.Order{
			ID:     orderID,
			Status: "pending_payment",
			Amount: 1000,
			OrderItems: []models.OrderItem{
				{ProductID: itemID, Quantity: 1, Price: 1000},
			},
		},
	}

	var confirmCalls int32
	inventoryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&confirmCalls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer inventoryServer.Close()

	productServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer productServer.Close()

	inventoryClient := NewInventoryClient(inventoryServer.URL)
	snsPublisher := &countingSNSPublisher{}

	consumer := NewSQSPaymentConsumer(nil, repo, inventoryClient, nil, snsPublisher, "arn:aws:sns:local:123456789012:notifications", productServer.URL)

	body := `{"type":"payment_succeeded","order_id":"` + orderID.String() + `","user_id":"` + uuid.New().String() + `"}`

	if err := consumer.handleMessage(context.Background(), body); err != nil {
		t.Fatalf("first delivery returned error: %v", err)
	}
	// Redelivery of the same event: status transition loses the race since the
	// order is already "paid", so side effects must not repeat.
	if err := consumer.handleMessage(context.Background(), body); err != nil {
		t.Fatalf("second (duplicate) delivery returned error: %v", err)
	}

	if got := atomic.LoadInt32(&confirmCalls); got != 1 {
		t.Fatalf("expected inventory confirm called once, got %d", got)
	}
	if got := atomic.LoadInt32(&snsPublisher.calls); got != 1 {
		t.Fatalf("expected notification published once, got %d", got)
	}
	repo.mu.Lock()
	status := repo.order.Status
	repo.mu.Unlock()
	if status != "paid" {
		t.Fatalf("expected order status paid, got %s", status)
	}
}
