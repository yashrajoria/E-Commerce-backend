package services

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yashrajoria/common/events"
	"go.uber.org/zap"
	"order-service/models"
)

type mockPriceDropRepo struct {
	items        []models.OrderItem
	recordedEvts []models.OutboxEvent
}

func (m *mockPriceDropRepo) FindEligibleOrderItems(ctx context.Context, productID uuid.UUID, cutoff time.Time) ([]models.OrderItem, error) {
	return m.items, nil
}

func (m *mockPriceDropRepo) RecordRefundOutboxEvents(ctx context.Context, events []models.OutboxEvent) error {
	m.recordedEvts = append(m.recordedEvts, events...)
	return nil
}

func TestPriceDropService_ZeroOrNegativeDelta(t *testing.T) {
	repo := &mockPriceDropRepo{}
	svc := NewPriceDropService(repo, "test-notif-topic", zap.NewNop())
	ctx := context.Background()

	res, err := svc.ProcessPriceDrop(ctx, events.ProductPriceDecreasedEvent{
		ProductID:  uuid.New().String(),
		OldPrice:   100.0,
		NewPrice:   100.0,
		DeltaPrice: 0.0,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, res.EligibleOrdersCount)
	assert.Empty(t, repo.recordedEvts)
}

func TestPriceDropService_EligibleOrdersCalculations(t *testing.T) {
	prodID := uuid.New()
	order1ID := uuid.New()
	order2ID := uuid.New()

	repo := &mockPriceDropRepo{
		items: []models.OrderItem{
			{
				ID:        uuid.New(),
				OrderID:   order1ID,
				ProductID: prodID,
				Quantity:  1,
				Price:     10000,
			},
			{
				ID:        uuid.New(),
				OrderID:   order2ID,
				ProductID: prodID,
				Quantity:  2,
				Price:     10000,
			},
		},
	}

	svc := NewPriceDropService(repo, "arn:aws:sns:local:123456789012:notifications", zap.NewNop())
	ctx := context.Background()

	// Price dropped from $100 to $85 (delta $15)
	evt := events.NewProductPriceDecreasedEvent(prodID.String(), 100.0, 85.0, time.Now().Unix())
	res, err := svc.ProcessPriceDrop(ctx, evt)

	require.NoError(t, err)
	assert.Equal(t, 2, res.EligibleOrdersCount)
	// Order 1: 1 * $15 = $15
	// Order 2: 2 * $15 = $30
	// Total: $45.00
	assert.Equal(t, 45.00, res.TotalRefundAmount)
	assert.Equal(t, 2, res.OutboxEventsCreated)
	require.Len(t, repo.recordedEvts, 2)

	// Check Outbox Event 1
	e1 := repo.recordedEvts[0]
	assert.Equal(t, "order", e1.AggregateType)
	assert.Equal(t, order1ID, e1.AggregateID)
	assert.Equal(t, "order.price_drop_refund.requested", e1.EventType)
	assert.Equal(t, models.OutboxStatusPending, e1.Status)

	var payload1 map[string]interface{}
	err = json.Unmarshal(e1.Payload, &payload1)
	require.NoError(t, err)
	assert.Equal(t, 1500.0, payload1["refund_amount_cents"]) // 1500 cents = $15.00
	assert.Equal(t, 15.0, payload1["delta_price"])

	// Check Outbox Event 2
	e2 := repo.recordedEvts[1]
	assert.Equal(t, order2ID, e2.AggregateID)
	var payload2 map[string]interface{}
	err = json.Unmarshal(e2.Payload, &payload2)
	require.NoError(t, err)
	assert.Equal(t, 3000.0, payload2["refund_amount_cents"]) // 3000 cents = $30.00
}
