package controllers

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"catalog-service/cart/models"
	productservices "catalog-service/services"

	"github.com/google/uuid"
)

func TestBuildCheckoutEventCapturesCorrelationID(t *testing.T) {
	event := buildCheckoutEvent("user-1", &models.Cart{Items: []models.CartItem{{ProductID: "product-1", Quantity: 2}}}, "order-1", "idem-1", "corr-1")
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var got models.CheckoutEvent
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if got.CorrelationID != "corr-1" {
		t.Fatalf("correlation_id=%q", got.CorrelationID)
	}
}

func TestBuildCheckoutEventAllowsFallbackCorrelationID(t *testing.T) {
	event := buildCheckoutEvent("user-1", &models.Cart{}, "order-1", "", "generated-correlation")
	if event.CorrelationID == "" {
		t.Fatal("expected a correlation ID at the cart entry point")
	}
}

type stubValidator struct {
	calls int32
	found map[string]bool
	err   error
}

func (s *stubValidator) GetProductsInternal(_ context.Context, ids []uuid.UUID) ([]*productservices.ProductInternalDTO, error) {
	atomic.AddInt32(&s.calls, 1)
	if s.err != nil {
		return nil, s.err
	}
	var out []*productservices.ProductInternalDTO
	for _, id := range ids {
		if s.found[id.String()] {
			out = append(out, &productservices.ProductInternalDTO{ID: id})
		}
	}
	return out, nil
}

func TestValidateProductsBatchUsesOneCallAndFlagsInvalid(t *testing.T) {
	validID := uuid.NewString()
	validator := &stubValidator{found: map[string]bool{validID: true}}
	controller := &CartController{Validator: validator}

	invalid, err := controller.validateProductsBatch(context.Background(), []string{validID, validID, "not-a-uuid", uuid.NewString()})
	if err != nil {
		t.Fatalf("batch validation failed: %v", err)
	}
	if len(invalid) != 2 {
		t.Fatalf("expected 2 invalid ids, got %v", invalid)
	}
	if atomic.LoadInt32(&validator.calls) != 1 {
		t.Fatalf("expected one validator call, got %d", validator.calls)
	}
}
