package repository

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"catalog-service/internal/pgtest"
	"catalog-service/inventory/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMergeItems_SumsDuplicatesAndSortsByID(t *testing.T) {
	a, b := "00000000-0000-4000-8000-00000000000a", "00000000-0000-4000-8000-00000000000b"
	lines, err := mergeItems([]models.ReserveItem{{ProductID: b, Quantity: 1}, {ProductID: a, Quantity: 2}, {ProductID: b, Quantity: 4}})
	require.NoError(t, err)
	require.Len(t, lines, 2)
	assert.Equal(t, a, lines[0].id.String())
	assert.Equal(t, 2, lines[0].qty)
	assert.Equal(t, 5, lines[1].qty)

	_, err = mergeItems([]models.ReserveItem{{ProductID: "nope", Quantity: 1}})
	assert.Error(t, err)
}

// seedStock creates a product + inventory row and registers cleanup.
func seedStock(t *testing.T, db *gorm.DB, r *PGInventoryRepository, available int) string {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO catalog.products (id, name, sku, price) VALUES (?, 'pgt-inv', ?, 1)`,
		id, "PGT-"+id.String()).Error)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM catalog.stock_reservations WHERE product_id = ?`, id)
		db.Exec(`DELETE FROM catalog.inventory WHERE product_id = ?`, id)
		db.Exec(`DELETE FROM catalog.products WHERE id = ?`, id)
	})
	_, err := r.AddStock(context.Background(), id.String(), available, 1)
	require.NoError(t, err)
	return id.String()
}

func levels(t *testing.T, r *PGInventoryRepository, id string) (available, reserved int) {
	t.Helper()
	inv, err := r.Get(context.Background(), id)
	require.NoError(t, err)
	return inv.Available, inv.Reserved
}

func TestPGInventory_AddStockUpserts(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGInventoryRepository(db)
	id := seedStock(t, db, r, 10)

	inv, err := r.AddStock(context.Background(), id, 5, 3)
	require.NoError(t, err)
	assert.Equal(t, 15, inv.Available)
	assert.Equal(t, 3, inv.Threshold)
	assert.Equal(t, 0, inv.Reserved)

	_, err = r.Get(context.Background(), "not-a-uuid")
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = r.Get(context.Background(), uuid.NewString())
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPGInventory_ReserveConfirmLifecycleIsIdempotent(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGInventoryRepository(db)
	ctx := context.Background()
	id := seedStock(t, db, r, 10)
	items := []models.ReserveItem{{ProductID: id, Quantity: 3}}
	order := "order-" + uuid.NewString()

	require.NoError(t, r.ReserveAll(ctx, order, items))
	a, rs := levels(t, r, id)
	assert.Equal(t, []int{7, 3}, []int{a, rs})
	inv, _ := r.Get(ctx, id)
	assert.Equal(t, map[string]int{order: 3}, inv.OrderReservations)

	require.NoError(t, r.ReserveAll(ctx, order, items), "replay is a no-op")
	a, rs = levels(t, r, id)
	assert.Equal(t, []int{7, 3}, []int{a, rs})

	require.NoError(t, r.ConfirmAll(ctx, order, items))
	a, rs = levels(t, r, id)
	assert.Equal(t, []int{7, 0}, []int{a, rs})
	inv, _ = r.Get(ctx, id)
	assert.Empty(t, inv.OrderReservations)

	require.NoError(t, r.ConfirmAll(ctx, order, items), "confirm replay is a no-op")
	assert.ErrorIs(t, r.ReleaseAll(ctx, order, items), ErrNoReservation, "cannot release a confirmed reservation")
}

func TestPGInventory_ReleaseRestoresAvailable(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGInventoryRepository(db)
	ctx := context.Background()
	id := seedStock(t, db, r, 10)
	items := []models.ReserveItem{{ProductID: id, Quantity: 4}}
	order := "order-" + uuid.NewString()

	require.NoError(t, r.ReserveAll(ctx, order, items))
	require.NoError(t, r.ReleaseAll(ctx, order, items))
	a, rs := levels(t, r, id)
	assert.Equal(t, []int{10, 0}, []int{a, rs})
	require.NoError(t, r.ReleaseAll(ctx, order, items), "release replay is a no-op")
	assert.ErrorIs(t, r.ConfirmAll(ctx, order, items), ErrNoReservation, "cannot confirm a released reservation")
	assert.ErrorIs(t, r.ReleaseAll(ctx, "order-"+uuid.NewString(), items), ErrNoReservation, "never reserved")
}

func TestPGInventory_ReserveRejectsInsufficientAndConflicts(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGInventoryRepository(db)
	ctx := context.Background()
	a := seedStock(t, db, r, 10)
	b := seedStock(t, db, r, 1)
	order := "order-" + uuid.NewString()

	// Second line is short: the whole transaction rolls back, line 1 untouched.
	err := r.ReserveAll(ctx, order, []models.ReserveItem{{ProductID: a, Quantity: 1}, {ProductID: b, Quantity: 2}})
	assert.ErrorIs(t, err, ErrInsufficientStock)
	av, rs := levels(t, r, a)
	assert.Equal(t, []int{10, 0}, []int{av, rs})

	// Unknown product behaves like zero stock.
	err = r.ReserveAll(ctx, order, []models.ReserveItem{{ProductID: uuid.NewString(), Quantity: 1}})
	assert.ErrorIs(t, err, ErrInsufficientStock)

	// Same order, same product, different quantity is a conflict, not a replay.
	require.NoError(t, r.ReserveAll(ctx, order, []models.ReserveItem{{ProductID: a, Quantity: 2}}))
	err = r.ReserveAll(ctx, order, []models.ReserveItem{{ProductID: a, Quantity: 3}})
	assert.ErrorIs(t, err, ErrDuplicateReservation)
}

func TestPGInventory_RestockBatchGetCheckUpdateList(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGInventoryRepository(db)
	ctx := context.Background()
	id := seedStock(t, db, r, 5)

	require.NoError(t, r.RestockAll(ctx, "o", []models.ReserveItem{{ProductID: id, Quantity: 2}}))
	av, _ := levels(t, r, id)
	assert.Equal(t, 7, av)
	assert.ErrorIs(t, r.RestockAll(ctx, "o", []models.ReserveItem{{ProductID: uuid.NewString(), Quantity: 1}}), ErrNotFound)

	m, err := r.BatchGet(ctx, []string{id, id, uuid.NewString(), "bad"})
	require.NoError(t, err)
	require.Len(t, m, 1)
	assert.Equal(t, 7, m[id].Available)

	res, err := r.CheckStock(ctx, id, 8)
	require.NoError(t, err)
	assert.False(t, res.IsSufficient)
	res, err = r.CheckStock(ctx, uuid.NewString(), 1)
	require.NoError(t, err)
	assert.False(t, res.IsSufficient)

	require.NoError(t, r.Update(ctx, id, map[string]interface{}{"available": 20, "threshold": 4, "updated_at": "ignored"}))
	inv, _ := r.Get(ctx, id)
	assert.Equal(t, 20, inv.Available)
	assert.Equal(t, 4, inv.Threshold)
	assert.ErrorIs(t, r.Update(ctx, uuid.NewString(), map[string]interface{}{"available": 1}), ErrNotFound)

	page, err := r.ListAll(ctx, 1, 0)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(page), 1)
}

func TestPGInventory_ConcurrentReservesNeverOversell(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGInventoryRepository(db)
	ctx := context.Background()
	id := seedStock(t, db, r, 10)

	var ok, short int32
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := r.ReserveAll(ctx, "order-"+uuid.NewString(), []models.ReserveItem{{ProductID: id, Quantity: 1}})
			switch {
			case err == nil:
				atomic.AddInt32(&ok, 1)
			case assert.ErrorIs(t, err, ErrInsufficientStock):
				atomic.AddInt32(&short, 1)
			}
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 10, ok)
	assert.EqualValues(t, 15, short)
	av, rs := levels(t, r, id)
	assert.Equal(t, []int{0, 10}, []int{av, rs})
}

func TestPGInventory_OppositeOrderMultiProductReservesDoNotDeadlock(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGInventoryRepository(db)
	ctx := context.Background()
	a, b := seedStock(t, db, r, 1000), seedStock(t, db, r, 1000)

	var wg sync.WaitGroup
	for _, items := range [][]models.ReserveItem{
		{{ProductID: a, Quantity: 1}, {ProductID: b, Quantity: 1}},
		{{ProductID: b, Quantity: 1}, {ProductID: a, Quantity: 1}},
	} {
		wg.Add(1)
		go func(items []models.ReserveItem) {
			defer wg.Done()
			for i := 0; i < 30; i++ {
				assert.NoError(t, r.ReserveAll(ctx, "order-"+uuid.NewString(), items))
			}
		}(items)
	}
	wg.Wait()
}
