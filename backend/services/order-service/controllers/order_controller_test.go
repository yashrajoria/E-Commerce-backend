package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"order-service/middleware"
	"order-service/models"
	paymentservices "order-service/payment/services"
	repositories "order-service/repository"
	"order-service/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type mockOrderRepo struct {
	repositories.OrderRepository
	mu    sync.Mutex
	order models.Order
	err   error
}

func (m *mockOrderRepo) FindByID(ctx context.Context, orderID uuid.UUID) (*models.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	o := m.order
	return &o, nil
}

func (m *mockOrderRepo) UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, fromStatus, toStatus string, extra map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.order.Status != fromStatus {
		return repositories.ErrStatusConflict
	}
	m.order.Status = toStatus
	return nil
}

type mockRefunder struct {
	calls        int
	lastOrderID  uuid.UUID
	lastAmount   *int
	lastReason   string
	lastIdemKey  string
	returnErr    error
	returnResult *paymentservices.RefundDetails
}

func (m *mockRefunder) RefundOrder(ctx context.Context, orderID uuid.UUID) error {
	m.calls++
	m.lastOrderID = orderID
	return m.returnErr
}

func (m *mockRefunder) RefundPayment(ctx context.Context, orderID uuid.UUID, amount *int, reason string, idempotencyKey string) (*paymentservices.RefundDetails, error) {
	m.calls++
	m.lastOrderID = orderID
	m.lastAmount = amount
	m.lastReason = reason
	m.lastIdemKey = idempotencyKey
	if m.returnErr != nil {
		return nil, m.returnErr
	}
	if m.returnResult != nil {
		return m.returnResult, nil
	}
	amt := 5000
	if amount != nil {
		amt = *amount
	}
	return &paymentservices.RefundDetails{
		RefundID:      "re_test_mock",
		Amount:        amt,
		Currency:      "usd",
		Status:        "succeeded",
		PaymentID:     uuid.NewString(),
		UserID:        uuid.NewString(),
		CorrelationID: "corr-1",
		RefundedAt:    time.Now().UTC(),
	}, nil
}

func setupTestRouter(repo *mockOrderRepo, refunder services.PaymentRefunder) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	svc := services.NewOrderServiceSQS(repo, nil, "", "", nil)
	if refunder != nil {
		svc.SetRefunder(refunder)
	}
	ctrl := NewOrderController(svc)
	orderRoutes := r.Group("/orders")
	orderRoutes.Use(middleware.AuthMiddleware())
	orderRoutes.POST("/:id/refund", ctrl.RefundOrder)
	return r
}

func TestRefundOrderRoute_SuccessFullRefund(t *testing.T) {
	orderID := uuid.New()
	userID := uuid.New()

	repo := &mockOrderRepo{
		order: models.Order{
			ID:       orderID,
			UserID:   userID,
			Status:   "paid",
			Amount:   5000,
			Currency: "usd",
		},
	}
	ref := &mockRefunder{}
	r := setupTestRouter(repo, ref)

	req, _ := http.NewRequest("POST", "/orders/"+orderID.String()+"/refund", nil)
	req.Header.Set("X-User-ID", userID.String())
	req.Header.Set("X-User-Role", "customer")
	req.Header.Set("Idempotency-Key", "refund-req-001")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	orderMap, ok := resp["order"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing order in response: %v", resp)
	}
	if orderMap["Status"] != "refunded" && orderMap["status"] != "refunded" {
		t.Fatalf("expected order status 'refunded', got %v", orderMap["Status"])
	}
	if ref.calls != 1 {
		t.Fatalf("expected refunder called once, got %d", ref.calls)
	}
	if ref.lastIdemKey != "refund-req-001" {
		t.Fatalf("expected idempotency key propagated, got %s", ref.lastIdemKey)
	}
}

func TestRefundOrderRoute_SuccessPartialRefund(t *testing.T) {
	orderID := uuid.New()
	userID := uuid.New()

	repo := &mockOrderRepo{
		order: models.Order{
			ID:       orderID,
			UserID:   userID,
			Status:   "fulfilled",
			Amount:   10000,
			Currency: "usd",
		},
	}
	ref := &mockRefunder{}
	r := setupTestRouter(repo, ref)

	bodyBytes, _ := json.Marshal(map[string]interface{}{
		"amount": 3500,
		"reason": "item damaged in transit",
	})
	req, _ := http.NewRequest("POST", "/orders/"+orderID.String()+"/refund", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", userID.String())

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
	if ref.calls != 1 {
		t.Fatalf("expected refunder called once, got %d", ref.calls)
	}
	if ref.lastAmount == nil || *ref.lastAmount != 3500 {
		t.Fatalf("expected amount 3500, got %v", ref.lastAmount)
	}
	if ref.lastReason != "item damaged in transit" {
		t.Fatalf("expected reason propagated, got %s", ref.lastReason)
	}
}

func TestRefundOrderRoute_RejectsInvalidStates(t *testing.T) {
	orderID := uuid.New()
	userID := uuid.New()

	// 1. Pending order
	repo := &mockOrderRepo{
		order: models.Order{
			ID:     orderID,
			UserID: userID,
			Status: "pending_payment",
			Amount: 5000,
		},
	}
	r := setupTestRouter(repo, &mockRefunder{})

	req, _ := http.NewRequest("POST", "/orders/"+orderID.String()+"/refund", nil)
	req.Header.Set("X-User-ID", userID.String())

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for pending order, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Already refunded order
	repo.order.Status = "refunded"
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)

	if w2.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for already refunded order, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestRefundOrderRoute_MissingPayment(t *testing.T) {
	orderID := uuid.New()
	userID := uuid.New()

	repo := &mockOrderRepo{
		order: models.Order{
			ID:     orderID,
			UserID: userID,
			Status: "paid",
			Amount: 5000,
		},
	}
	ref := &mockRefunder{returnErr: paymentservices.ErrPaymentNotFound}
	r := setupTestRouter(repo, ref)

	req, _ := http.NewRequest("POST", "/orders/"+orderID.String()+"/refund", nil)
	req.Header.Set("X-User-ID", userID.String())

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for missing payment, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRefundOrderRoute_StripeFailure(t *testing.T) {
	orderID := uuid.New()
	userID := uuid.New()

	repo := &mockOrderRepo{
		order: models.Order{
			ID:     orderID,
			UserID: userID,
			Status: "paid",
			Amount: 5000,
		},
	}
	ref := &mockRefunder{returnErr: errors.New("Stripe API down")}
	r := setupTestRouter(repo, ref)

	req, _ := http.NewRequest("POST", "/orders/"+orderID.String()+"/refund", nil)
	req.Header.Set("X-User-ID", userID.String())

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 Bad Gateway for Stripe error, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRefundOrderRoute_UnauthorizedAndForbidden(t *testing.T) {
	orderID := uuid.New()
	ownerID := uuid.New()
	strangerID := uuid.New()

	repo := &mockOrderRepo{
		order: models.Order{
			ID:     orderID,
			UserID: ownerID,
			Status: "paid",
			Amount: 5000,
		},
	}
	r := setupTestRouter(repo, &mockRefunder{})

	// 1. Unauthenticated (no X-User-ID header)
	reqUnauth, _ := http.NewRequest("POST", "/orders/"+orderID.String()+"/refund", nil)
	wUnauth := httptest.NewRecorder()
	r.ServeHTTP(wUnauth, reqUnauth)
	if wUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", wUnauth.Code)
	}

	// 2. Stranger (non-owner, non-admin)
	reqStranger, _ := http.NewRequest("POST", "/orders/"+orderID.String()+"/refund", nil)
	reqStranger.Header.Set("X-User-ID", strangerID.String())
	reqStranger.Header.Set("X-User-Role", "customer")
	wStranger := httptest.NewRecorder()
	r.ServeHTTP(wStranger, reqStranger)
	if wStranger.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", wStranger.Code)
	}

	// 3. Admin is allowed
	reqAdmin, _ := http.NewRequest("POST", "/orders/"+orderID.String()+"/refund", nil)
	reqAdmin.Header.Set("X-User-ID", strangerID.String())
	reqAdmin.Header.Set("X-User-Role", "admin")
	wAdmin := httptest.NewRecorder()
	r.ServeHTTP(wAdmin, reqAdmin)
	if wAdmin.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin, got %d: %s", wAdmin.Code, wAdmin.Body.String())
	}
}

func TestRefundOrderRoute_InvalidIDAndMalformedBody(t *testing.T) {
	r := setupTestRouter(&mockOrderRepo{}, &mockRefunder{})

	// Invalid UUID
	reqInvalidID, _ := http.NewRequest("POST", "/orders/not-a-uuid/refund", nil)
	reqInvalidID.Header.Set("X-User-ID", uuid.NewString())
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, reqInvalidID)
	if w1.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for bad UUID, got %d", w1.Code)
	}

	// Malformed Idempotency Key
	orderID := uuid.New()
	reqBadKey, _ := http.NewRequest("POST", "/orders/"+orderID.String()+"/refund", nil)
	reqBadKey.Header.Set("X-User-ID", uuid.NewString())
	reqBadKey.Header.Set("Idempotency-Key", "invalid spaces and @@@@")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, reqBadKey)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for bad idempotency key, got %d", w2.Code)
	}
}
