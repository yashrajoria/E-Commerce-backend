package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"order-service/models"
	repositories "order-service/repository"

	"github.com/google/uuid"
)

type checkoutOutboxRepo struct {
	repositories.OrderRepository
	createErr  error
	findResult *models.Order
	createCall int32
	events     []models.OutboxEvent
}

func (r *checkoutOutboxRepo) CreateWithOutbox(_ context.Context, _ *models.Order, events []models.OutboxEvent) error {
	atomic.AddInt32(&r.createCall, 1)
	r.events = append([]models.OutboxEvent(nil), events...)
	return r.createErr
}

func (r *checkoutOutboxRepo) FindByIdempotencyKey(context.Context, string) (*models.Order, error) {
	return r.findResult, nil
}

func TestCheckoutOutboxFailureReleasesReservedStock(t *testing.T) {
	productID := uuid.New()
	orderID := uuid.New()
	var reserveCalls, releaseCalls int32
	inventoryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/inventory/reserve":
			atomic.AddInt32(&reserveCalls, 1)
		case "/inventory/release":
			atomic.AddInt32(&releaseCalls, 1)
		default:
			http.NotFound(w, req)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer inventoryServer.Close()

	productServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"id":"` + productID.String() + `","name":"Widget","price":12.5}`))
	}))
	defer productServer.Close()

	repo := &checkoutOutboxRepo{createErr: errors.New("transaction rolled back")}
	consumer := NewSQSCheckoutConsumer(nil, nil, repo, NewInventoryClient(inventoryServer.URL), nil, productServer.URL, nil, "", nil, "usd")
	body := `{"event":"checkout.requested","user_id":"` + uuid.NewString() + `","order_id":"` + orderID.String() + `","idempotency_key":"checkout-1","items":[{"product_id":"` + productID.String() + `","quantity":1}]}`

	if err := consumer.handleMessage(context.Background(), body); err == nil {
		t.Fatal("expected outbox transaction error")
	}
	if atomic.LoadInt32(&reserveCalls) != 1 || atomic.LoadInt32(&releaseCalls) != 1 {
		t.Fatalf("expected reserve and compensating release, reserve=%d release=%d", reserveCalls, releaseCalls)
	}
	if atomic.LoadInt32(&repo.createCall) != 1 || len(repo.events) != 2 {
		t.Fatalf("expected one transactional create with two outbox events, calls=%d events=%d", repo.createCall, len(repo.events))
	}
}
