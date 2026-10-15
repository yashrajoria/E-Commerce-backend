package services

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"order-service/credit"
	"order-service/models"
	paymentservices "order-service/payment/services"
	repositories "order-service/repository"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

// fakeInventoryReleaser records ReleaseStock/RestockStock calls for cancellation tests.
type fakeInventoryReleaser struct {
	mu               sync.Mutex
	calls            int
	restockCalls     int
	lastOrder        string
	lastItems        []ReserveItem
	lastRestockOrder string
	lastRestockItems []ReserveItem
	releaseErr       error
	restockErr       error
}

func (f *fakeInventoryReleaser) ReleaseStock(ctx context.Context, orderID string, items []ReserveItem) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastOrder = orderID
	f.lastItems = items
	return f.releaseErr
}

func (f *fakeInventoryReleaser) RestockStock(ctx context.Context, orderID string, items []ReserveItem) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restockCalls++
	f.lastRestockOrder = orderID
	f.lastRestockItems = items
	return f.restockErr
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
	if releaser.restockCalls != 0 {
		t.Fatalf("expected no RestockStock for pending_payment cancel, got %d", releaser.restockCalls)
	}
	if releaser.lastOrder != orderID.String() {
		t.Fatalf("unexpected order id passed to ReleaseStock: %s", releaser.lastOrder)
	}
}

func TestCancelOrder_PaidRestocksInsteadOfRelease(t *testing.T) {
	orderID := uuid.New()
	productID := uuid.New()
	repo := &fakeOrderRepo{
		order: models.Order{
			ID:     orderID,
			Status: "paid",
			OrderItems: []models.OrderItem{
				{ProductID: productID, Quantity: 3},
			},
		},
	}
	releaser := &fakeInventoryReleaser{}
	svc := NewOrderServiceSQS(repo, nil, "", "", releaser)

	order, err := svc.CancelOrder(context.Background(), orderID, "admin-1", "refund")
	if err != nil {
		t.Fatalf("CancelOrder returned error: %v", err)
	}
	if order.Status != "cancelled" {
		t.Fatalf("expected status cancelled, got %s", order.Status)
	}
	if releaser.restockCalls != 1 {
		t.Fatalf("expected RestockStock called once for paid cancel, got %d", releaser.restockCalls)
	}
	if releaser.calls != 0 {
		t.Fatalf("expected no ReleaseStock for paid cancel, got %d", releaser.calls)
	}
	if releaser.lastRestockOrder != orderID.String() {
		t.Fatalf("unexpected order id passed to RestockStock: %s", releaser.lastRestockOrder)
	}
	if len(releaser.lastRestockItems) != 1 || releaser.lastRestockItems[0].Quantity != 3 {
		t.Fatalf("unexpected restock items: %+v", releaser.lastRestockItems)
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

type fakeRefunder struct {
	err   error
	calls int
}

func (f *fakeRefunder) RefundOrder(context.Context, uuid.UUID) error { f.calls++; return f.err }

func TestCancelOrder_PaidRefundsBeforeCancelling(t *testing.T) {
	orderID := uuid.New()
	repo := &fakeOrderRepo{order: models.Order{ID: orderID, Status: "paid", OrderItems: []models.OrderItem{{ProductID: uuid.New(), Quantity: 1}}}}
	refunder := &fakeRefunder{}
	svc := NewOrderServiceSQS(repo, nil, "", "", &fakeInventoryReleaser{})
	svc.SetRefunder(refunder)

	if _, err := svc.CancelOrder(context.Background(), orderID, "admin-1", "r"); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if refunder.calls != 1 {
		t.Fatalf("expected one refund, got %d", refunder.calls)
	}
}

func TestCancelOrder_RefundFailureKeepsOrderPaid(t *testing.T) {
	orderID := uuid.New()
	repo := &fakeOrderRepo{order: models.Order{ID: orderID, Status: "paid"}}
	svc := NewOrderServiceSQS(repo, nil, "", "", &fakeInventoryReleaser{})
	svc.SetRefunder(&fakeRefunder{err: errors.New("stripe down")})

	_, serr := svc.CancelOrder(context.Background(), orderID, "admin-1", "r")
	if serr == nil || serr.StatusCode != 502 {
		t.Fatalf("expected 502 ServiceError, got %v", serr)
	}
	if repo.order.Status != "paid" {
		t.Fatalf("order must stay paid when refund fails, got %s", repo.order.Status)
	}
}

type fakeDetailedRefunder struct {
	calls        int
	lastOrderID  uuid.UUID
	lastAmount   *int
	lastReason   string
	lastIdemKey  string
	returnErr    error
	returnResult *paymentservices.RefundDetails
}

func (f *fakeDetailedRefunder) RefundOrder(ctx context.Context, orderID uuid.UUID) error {
	f.calls++
	f.lastOrderID = orderID
	return f.returnErr
}

func (f *fakeDetailedRefunder) RefundPayment(ctx context.Context, orderID uuid.UUID, amount *int, reason string, idempotencyKey string) (*paymentservices.RefundDetails, error) {
	f.calls++
	f.lastOrderID = orderID
	f.lastAmount = amount
	f.lastReason = reason
	f.lastIdemKey = idempotencyKey
	if f.returnErr != nil {
		return nil, f.returnErr
	}
	if f.returnResult != nil {
		return f.returnResult, nil
	}
	amt := 5000
	if amount != nil {
		amt = *amount
	}
	return &paymentservices.RefundDetails{
		RefundID:      "re_test_999",
		Amount:        amt,
		Currency:      "usd",
		Status:        "succeeded",
		PaymentID:     uuid.NewString(),
		UserID:        uuid.NewString(),
		CorrelationID: "corr-test",
		RefundedAt:    time.Now().UTC(),
	}, nil
}

func TestRefundOrder_TransitionsToRefunded(t *testing.T) {
	orderID := uuid.New()
	userID := uuid.New()
	productID := uuid.New()

	repo := &fakeOrderRepo{
		order: models.Order{
			ID:       orderID,
			UserID:   userID,
			Status:   "paid",
			Amount:   5000,
			Currency: "usd",
			OrderItems: []models.OrderItem{
				{ProductID: productID, Quantity: 2},
			},
		},
	}
	releaser := &fakeInventoryReleaser{}
	refunder := &fakeDetailedRefunder{}
	fakePublisher := &fakeSNSPublisher{}

	svc := NewOrderServiceSQS(repo, fakePublisher, "arn:aws:sns:local:123456789012:payments", "", releaser)
	svc.SetRefunder(refunder)

	partialAmt := 2500
	order, serr := svc.RefundOrder(context.Background(), orderID, userID.String(), "customer", &RefundOrderRequest{
		Reason: "customer_return",
		Amount: &partialAmt,
	})
	if serr != nil {
		t.Fatalf("expected successful refund, got error: %v", serr)
	}

	if order.Status != "refunded" {
		t.Fatalf("expected order status to be 'refunded', got %s", order.Status)
	}
	if refunder.calls != 1 {
		t.Fatalf("expected refunder called once, got %d", refunder.calls)
	}
	if *refunder.lastAmount != 2500 || refunder.lastReason != "customer_return" {
		t.Fatalf("unexpected refunder args: amount=%v reason=%s", refunder.lastAmount, refunder.lastReason)
	}
	if releaser.restockCalls != 1 {
		t.Fatalf("expected 1 restock call, got %d", releaser.restockCalls)
	}
	if fakePublisher.lastTopic != "arn:aws:sns:local:123456789012:payments" {
		t.Fatalf("unexpected published topic: %s", fakePublisher.lastTopic)
	}

	var pubEvt models.PaymentEvent
	if err := json.Unmarshal(fakePublisher.lastMessage, &pubEvt); err != nil {
		t.Fatalf("unmarshal refund event: %v", err)
	}
	if pubEvt.Type != "payment_refunded" || pubEvt.Amount != 2500 {
		t.Fatalf("unexpected published event: %+v", pubEvt)
	}
}

func TestRefundOrder_FulfilledOrderEligible(t *testing.T) {
	orderID := uuid.New()
	userID := uuid.New()

	for _, status := range []string{"fulfilled", "completed", "PAID", "FULFILLED"} {
		repo := &fakeOrderRepo{
			order: models.Order{
				ID:       orderID,
				UserID:   userID,
				Status:   status,
				Amount:   3000,
				Currency: "usd",
			},
		}
		refunder := &fakeDetailedRefunder{}
		svc := NewOrderServiceSQS(repo, nil, "", "", nil)
		svc.SetRefunder(refunder)

		order, serr := svc.RefundOrder(context.Background(), orderID, userID.String(), "", &RefundOrderRequest{})
		if serr != nil {
			t.Fatalf("status %s should be eligible for refund, got error: %v", status, serr)
		}
		if order.Status != "refunded" {
			t.Fatalf("expected status 'refunded', got %s", order.Status)
		}
	}
}

func TestRefundOrder_RejectsInvalidStates(t *testing.T) {
	orderID := uuid.New()
	userID := uuid.New()

	// 1. Pending payment
	repoPending := &fakeOrderRepo{
		order: models.Order{ID: orderID, UserID: userID, Status: "pending_payment"},
	}
	svc := NewOrderServiceSQS(repoPending, nil, "", "", nil)
	svc.SetRefunder(&fakeDetailedRefunder{})
	_, serr := svc.RefundOrder(context.Background(), orderID, userID.String(), "", &RefundOrderRequest{})
	if serr == nil || serr.StatusCode != 409 {
		t.Fatalf("expected 409 for pending_payment, got %v", serr)
	}

	// 2. Already refunded
	repoRefunded := &fakeOrderRepo{
		order: models.Order{ID: orderID, UserID: userID, Status: "refunded", Amount: 2000, RefundedAmount: 2000},
	}
	svc = NewOrderServiceSQS(repoRefunded, nil, "", "", nil)
	svc.SetRefunder(&fakeDetailedRefunder{})
	_, serr = svc.RefundOrder(context.Background(), orderID, userID.String(), "", &RefundOrderRequest{})
	if serr == nil || serr.StatusCode != 409 || serr.Message != "order is already fully refunded" {
		t.Fatalf("expected 409 'order is already fully refunded', got %v", serr)
	}

	// 3. Cancelled
	repoCancelled := &fakeOrderRepo{
		order: models.Order{ID: orderID, UserID: userID, Status: "cancelled"},
	}
	svc = NewOrderServiceSQS(repoCancelled, nil, "", "", nil)
	svc.SetRefunder(&fakeDetailedRefunder{})
	_, serr = svc.RefundOrder(context.Background(), orderID, userID.String(), "", &RefundOrderRequest{})
	if serr == nil || serr.StatusCode != 409 {
		t.Fatalf("expected 409 for cancelled order, got %v", serr)
	}
}

func TestRefundOrder_Authorization(t *testing.T) {
	orderID := uuid.New()
	ownerID := uuid.New()
	strangerID := uuid.New()

	repo := &fakeOrderRepo{
		order: models.Order{ID: orderID, UserID: ownerID, Status: "paid", Amount: 2000},
	}
	svc := NewOrderServiceSQS(repo, nil, "", "", nil)
	svc.SetRefunder(&fakeDetailedRefunder{})

	// Stranger without admin role is forbidden
	_, serr := svc.RefundOrder(context.Background(), orderID, strangerID.String(), "customer", &RefundOrderRequest{})
	if serr == nil || serr.StatusCode != 403 {
		t.Fatalf("expected 403 Forbidden for stranger, got %v", serr)
	}

	// Admin stranger is allowed
	_, serr = svc.RefundOrder(context.Background(), orderID, strangerID.String(), "admin", &RefundOrderRequest{})
	if serr != nil {
		t.Fatalf("expected admin to be allowed, got error: %v", serr)
	}
}

func TestRefundOrder_InvalidAmount(t *testing.T) {
	orderID := uuid.New()
	userID := uuid.New()

	repo := &fakeOrderRepo{
		order: models.Order{ID: orderID, UserID: userID, Status: "paid", Amount: 5000},
	}
	svc := NewOrderServiceSQS(repo, nil, "", "", nil)
	svc.SetRefunder(&fakeDetailedRefunder{})

	// Negative / zero amount
	neg := -100
	_, serr := svc.RefundOrder(context.Background(), orderID, userID.String(), "", &RefundOrderRequest{Amount: &neg})
	if serr == nil || serr.StatusCode != 400 {
		t.Fatalf("expected 400 for negative amount, got %v", serr)
	}

	// Amount > order amount
	huge := 6000
	_, serr = svc.RefundOrder(context.Background(), orderID, userID.String(), "", &RefundOrderRequest{Amount: &huge})
	if serr == nil || serr.StatusCode != 400 {
		t.Fatalf("expected 400 for amount > total, got %v", serr)
	}
}

func TestRefundOrder_PaymentMissing(t *testing.T) {
	orderID := uuid.New()
	userID := uuid.New()

	repo := &fakeOrderRepo{
		order: models.Order{ID: orderID, UserID: userID, Status: "paid", Amount: 5000},
	}
	refunder := &fakeDetailedRefunder{returnErr: paymentservices.ErrPaymentNotFound}
	svc := NewOrderServiceSQS(repo, nil, "", "", nil)
	svc.SetRefunder(refunder)

	_, serr := svc.RefundOrder(context.Background(), orderID, userID.String(), "", &RefundOrderRequest{})
	if serr == nil || serr.StatusCode != 404 {
		t.Fatalf("expected 404 when payment record missing, got %v", serr)
	}
	if repo.order.Status != "paid" {
		t.Fatalf("order status should stay paid on error, got %s", repo.order.Status)
	}
}

func TestRefundOrder_StripeFailure(t *testing.T) {
	orderID := uuid.New()
	userID := uuid.New()

	repo := &fakeOrderRepo{
		order: models.Order{ID: orderID, UserID: userID, Status: "paid", Amount: 5000},
	}
	refunder := &fakeDetailedRefunder{returnErr: errors.New("stripe service unavailable")}
	svc := NewOrderServiceSQS(repo, nil, "", "", nil)
	svc.SetRefunder(refunder)

	_, serr := svc.RefundOrder(context.Background(), orderID, userID.String(), "", &RefundOrderRequest{})
	if serr == nil || serr.StatusCode != 502 {
		t.Fatalf("expected 502 when Stripe fails, got %v", serr)
	}
	if repo.order.Status != "paid" {
		t.Fatalf("order status should stay paid on error, got %s", repo.order.Status)
	}
}

type fakeAllocRepo struct {
	allocs []models.OrderPaymentAllocation
}

func (f *fakeAllocRepo) Create(ctx context.Context, allocation *models.OrderPaymentAllocation) error {
	f.allocs = append(f.allocs, *allocation)
	return nil
}

func (f *fakeAllocRepo) FindByOrderID(ctx context.Context, orderID uuid.UUID) ([]models.OrderPaymentAllocation, error) {
	var result []models.OrderPaymentAllocation
	for _, a := range f.allocs {
		if a.OrderID == orderID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (f *fakeAllocRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	for i := range f.allocs {
		if f.allocs[i].ID == id {
			f.allocs[i].Status = status
			return nil
		}
	}
	return nil
}

func (f *fakeAllocRepo) UpdateRefunded(ctx context.Context, id uuid.UUID, refundedAmount int64) error {
	for i := range f.allocs {
		if f.allocs[i].ID == id {
			f.allocs[i].RefundedAmount = refundedAmount
			return nil
		}
	}
	return nil
}

func TestRefundOrder_AllocationSplit_ProportionalAndSubsequent(t *testing.T) {
	orderID := uuid.New()
	userID := uuid.New()
	storeID := uuid.New()

	order := models.Order{
		ID:             orderID,
		UserID:         userID,
		StoreID:        &storeID,
		Status:         "paid",
		Amount:         10000, // £100
		RefundedAmount: 0,
		Currency:       "gbp",
	}
	repo := &fakeOrderRepo{order: order}

	allocRepo := &fakeAllocRepo{
		allocs: []models.OrderPaymentAllocation{
			{
				ID:             uuid.New(),
				OrderID:        orderID,
				PaymentMethod:  "store_credit",
				Amount:         4000, // £40
				RefundedAmount: 0,
				Status:         "captured",
			},
			{
				ID:             uuid.New(),
				OrderID:        orderID,
				PaymentMethod:  "stripe",
				Amount:         6000, // £60
				RefundedAmount: 0,
				Status:         "captured",
			},
		},
	}

	// Use in-memory SQLite DB for credit service
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test sqlite: %v", err)
	}
	_ = db.AutoMigrate(&models.StoreCreditAccount{}, &models.StoreCreditLedgerEntry{})
	creditSvc := credit.NewService(db)

	stripeRefunder := &fakeDetailedRefunder{}
	svc := NewOrderServiceSQS(repo, nil, "", "", nil)
	svc.SetRefunder(stripeRefunder)
	svc.SetCreditService(creditSvc)
	svc.SetAllocationRepo(allocRepo)

	// 1. Partial refund of 5000 (£50)
	reqAmt := 5000
	updatedOrder, serr := svc.RefundOrder(context.Background(), orderID, userID.String(), "customer", &RefundOrderRequest{
		Amount: &reqAmt,
		Reason: "customer return partial",
	})
	if serr != nil {
		t.Fatalf("partial refund failed: %v", serr)
	}
	if updatedOrder.Status != "refunded" {
		t.Fatalf("expected status 'refunded', got %s", updatedOrder.Status)
	}
	if updatedOrder.RefundedAmount != 5000 {
		t.Fatalf("expected refunded amount 5000, got %d", updatedOrder.RefundedAmount)
	}

	// Verify allocations updated proportionally:
	// Credit: 4000 / 10000 * 5000 = 2000
	// Stripe: 6000 / 10000 * 5000 = 3000
	allocs, _ := allocRepo.FindByOrderID(context.Background(), orderID)
	for _, a := range allocs {
		if a.PaymentMethod == "store_credit" && a.RefundedAmount != 2000 {
			t.Fatalf("expected credit allocation refunded 2000, got %d", a.RefundedAmount)
		}
		if a.PaymentMethod == "stripe" && a.RefundedAmount != 3000 {
			t.Fatalf("expected stripe allocation refunded 3000, got %d", a.RefundedAmount)
		}
	}

	// Verify customer received 2000 store credit in balance
	avail, _, _, _ := creditSvc.Balance(context.Background(), storeID, userID)
	if avail != 2000 {
		t.Fatalf("expected customer credit balance 2000, got %d", avail)
	}

	// 2. Second partial refund of remaining 5000
	fullOrder, serr := svc.RefundOrder(context.Background(), orderID, userID.String(), "customer", &RefundOrderRequest{
		Reason: "return remaining items",
	})
	if serr != nil {
		t.Fatalf("second refund failed: %v", serr)
	}
	if fullOrder.Status != "refunded" {
		t.Fatalf("expected final status 'refunded', got %s", fullOrder.Status)
	}
	if fullOrder.RefundedAmount != 10000 {
		t.Fatalf("expected total refunded 10000, got %d", fullOrder.RefundedAmount)
	}

	// 3. Third refund attempt on fully refunded order must fail with 409
	_, serr = svc.RefundOrder(context.Background(), orderID, userID.String(), "customer", &RefundOrderRequest{})
	if serr == nil || serr.StatusCode != 409 {
		t.Fatalf("expected 409 on third refund attempt, got %v", serr)
	}
}
