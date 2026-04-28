package services

import (
	"context"
	"encoding/json"
	"testing"

	"order-service/models"

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
	svc := NewOrderServiceSQS(nil, fakePublisher, "arn:aws:sns:local:123456789012:orders", "")

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
