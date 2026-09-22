package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"cart-service/models"

	"github.com/gin-gonic/gin"
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

func TestValidateProductsBatchUsesOneRequestAndBothHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Method != http.MethodPost || r.URL.Path != "/products/internal/batch-validate" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Request-ID") != "req-1" || r.Header.Get("X-Correlation-ID") != "corr-1" {
			t.Fatalf("headers were not propagated: %v", r.Header)
		}
		_, _ = w.Write([]byte(`{"invalid_product_ids":[]}`))
	}))
	defer server.Close()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/checkout", nil)
	ctx.Request.Header.Set("X-Request-ID", "req-1")
	ctx.Request.Header.Set("X-Correlation-ID", "corr-1")
	controller := &CartController{}
	invalid, err := controller.validateProductsBatch(context.Background(), ctx, server.URL, []string{"p1", "p2"})
	if err != nil || len(invalid) != 0 {
		t.Fatalf("batch validation failed: invalid=%v err=%v", invalid, err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected one batch request, got %d", calls)
	}
}
