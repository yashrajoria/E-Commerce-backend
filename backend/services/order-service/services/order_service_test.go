package services

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"order-service/models"
	repositories "order-service/repository"

	"github.com/google/uuid"
)

type fakeSNSPublisher struct {
	lastTopic   string
	lastMessage []byte
	publishErr  error
}

func (f *fakeSNSPublisher) Publish(ctx context.Context, topicArn string, message []byte) error {
	f.lastTopic = topicArn
	f.lastMessage = append([]byte(nil), message...)
	return f.publishErr
}

func TestCreateOrderPropagatesIdempotencyKey(t *testing.T) {
	fakePublisher := &fakeSNSPublisher{}
	svc := NewOrderServiceSQS(nil, fakePublisher, "arn:aws:sns:local:123456789012:orders", "", nil)

	ctx := context.WithValue(context.Background(), IdempotencyKeyContextKey, "user-123:req-456")
	req := &CreateOrderRequest{
		Items: []struct {
			ProductID uuid.UUID `json:"product_id" binding:"required"`
			Quantity  int       `json:"quantity" binding:"required,min=1"`
		}{
			{ProductID: uuid.MustParse("11111111-1111-1111-1111-111111111111"), Quantity: 2},
		},
	}

	if err := svc.CreateOrder(ctx, "22222222-2222-2222-2222-222222222222", "user@example.com", req); err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	if fakePublisher.lastTopic != "arn:aws:sns:local:123456789012:orders" {
		t.Fatalf("unexpected topic: %s", fakePublisher.lastTopic)
	}

	var evt models.CheckoutEvent
	if err := json.Unmarshal(fakePublisher.lastMessage, &evt); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	if got := evt.IdempotencyKey; got != "user-123:req-456" {
		t.Fatalf("expected propagated idempotency key, got %q", got)
	}
	if evt.UserID != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("unexpected user id: %s", evt.UserID)
	}
	if len(evt.Items) != 1 {
		t.Fatalf("unexpected items length: %d", len(evt.Items))
	}
}

// fakeInventoryReleaser records ReleaseStock calls for cancellation tests.
type fakeInventoryReleaser struct {
	mu         sync.Mutex
	calls      int
	lastOrder  string
	lastItems  []ReserveItem
	releaseErr error
}

func (f *fakeInventoryReleaser) ReleaseStock(ctx context.Context, orderID string, items []ReserveItem) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastOrder = orderID
	f.lastItems = items
	return f.releaseErr
}

func TestCancelOrder_ReleasesStockOnSuccess(t *testing.T) {
	orderID := uuid.New()
	productID := uuid.New()
	repo := &fakeOrderRepo{
		order: models.Order{
			ID:     orderID,
			Status: "pending_payment",
			OrderItems: []models.OrderItem{
				{ProductID: productID, Quantity: 2},
			},
		},
	}
	releaser := &fakeInventoryReleaser{}
	svc := NewOrderServiceSQS(repo, nil, "", "", releaser)

	order, err := svc.CancelOrder(context.Background(), orderID, "admin-1", "customer request")
	if err != nil {
		t.Fatalf("CancelOrder returned error: %v", err)
	}
	if order.Status != "cancelled" {
		t.Fatalf("expected status cancelled, got %s", order.Status)
	}
	if releaser.calls != 1 {
		t.Fatalf("expected ReleaseStock called once, got %d", releaser.calls)
	}
	if releaser.lastOrder != orderID.String() {
		t.Fatalf("unexpected order id passed to ReleaseStock: %s", releaser.lastOrder)
	}
}

func TestCancelOrder_RejectsTerminalStatus(t *testing.T) {
	orderID := uuid.New()
	repo := &fakeOrderRepo{
		order: models.Order{ID: orderID, Status: "completed"},
	}
	releaser := &fakeInventoryReleaser{}
	svc := NewOrderServiceSQS(repo, nil, "", "", releaser)

	_, err := svc.CancelOrder(context.Background(), orderID, "admin-1", "")
	if err == nil {
		t.Fatal("expected error cancelling a completed order")
	}
	if err.StatusCode != 409 {
		t.Fatalf("expected 409, got %d", err.StatusCode)
	}
	if releaser.calls != 0 {
		t.Fatalf("expected no inventory release for rejected cancellation, got %d calls", releaser.calls)
	}
}

// raceyOrderRepo returns a stale "cancellable" snapshot from FindByID but
// simulates a concurrent writer having already changed the row's real status
// by the time UpdateOrderStatus's optimistic-locking WHERE clause runs.
type raceyOrderRepo struct {
	repositories.OrderRepository
	snapshot models.Order
}

func (r *raceyOrderRepo) FindByID(ctx context.Context, orderID uuid.UUID) (*models.Order, error) {
	o := r.snapshot
	return &o, nil
}

func (r *raceyOrderRepo) UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, fromStatus, toStatus string, extra map[string]interface{}) error {
	return repositories.ErrStatusConflict
}

func TestCancelOrder_StatusConflict(t *testing.T) {
	orderID := uuid.New()
	repo := &raceyOrderRepo{snapshot: models.Order{ID: orderID, Status: "paid"}}
	svc := NewOrderServiceSQS(repo, nil, "", "", nil)

	_, err := svc.CancelOrder(context.Background(), orderID, "admin-1", "")
	if err == nil {
		t.Fatal("expected status-conflict error")
	}
	if err.StatusCode != 409 {
		t.Fatalf("expected 409, got %d", err.StatusCode)
	}
}

var _ repositories.OrderRepository = (*fakeOrderRepo)(nil)
