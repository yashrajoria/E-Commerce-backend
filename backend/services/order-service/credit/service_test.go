package credit_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"order-service/credit"
	"order-service/models"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&models.StoreCreditAccount{},
		&models.StoreCreditLedgerEntry{},
		&models.OrderPaymentAllocation{},
	)
	require.NoError(t, err)
	return db
}

func TestGrant_SuccessAndIdempotency(t *testing.T) {
	db := setupTestDB(t)
	svc := credit.NewService(db)
	ctx := context.Background()

	storeID := uuid.New()
	userID := uuid.New()
	actor := uuid.New()

	// 1. Initial balance should be 0
	avail, held, cur, err := svc.Balance(ctx, storeID, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), avail)
	assert.Equal(t, int64(0), held)
	assert.Equal(t, "GBP", cur)

	// 2. Grant 500 minor units (£5.00)
	idemKey := "grant-1"
	err = svc.Grant(ctx, storeID, userID, 500, "GBP", "courtesy credit", idemKey, &actor, nil)
	require.NoError(t, err)

	avail, held, _, err = svc.Balance(ctx, storeID, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(500), avail)
	assert.Equal(t, int64(0), held)

	// 3. Retry same grant with same idempotency key - must not double credit
	err = svc.Grant(ctx, storeID, userID, 500, "GBP", "courtesy credit", idemKey, &actor, nil)
	require.NoError(t, err)

	avail, _, _, err = svc.Balance(ctx, storeID, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(500), avail)

	// 4. Verify ledger has exactly 1 entry
	entries, err := svc.Ledger(ctx, storeID, userID, 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "grant", entries[0].EntryType)
	assert.Equal(t, int64(500), entries[0].Amount)
	assert.Equal(t, int64(500), entries[0].AvailableDelta)
	assert.Equal(t, int64(0), entries[0].HeldDelta)
}

func TestDebit_SuccessInsufficientAndIdempotency(t *testing.T) {
	db := setupTestDB(t)
	svc := credit.NewService(db)
	ctx := context.Background()

	storeID := uuid.New()
	userID := uuid.New()
	actor := uuid.New()

	// Grant initial 1000
	require.NoError(t, svc.Grant(ctx, storeID, userID, 1000, "GBP", "initial", "grant-1", &actor, nil))

	// Try debiting 1500 -> ErrInsufficientCredit
	err := svc.Debit(ctx, storeID, userID, 1500, "GBP", "overcharge", "deb-1", &actor)
	assert.ErrorIs(t, err, credit.ErrInsufficientCredit)

	// Balance remains 1000
	avail, _, _, _ := svc.Balance(ctx, storeID, userID)
	assert.Equal(t, int64(1000), avail)

	// Valid debit 400
	err = svc.Debit(ctx, storeID, userID, 400, "GBP", "chargeback adjustment", "deb-2", &actor)
	require.NoError(t, err)

	avail, _, _, _ = svc.Balance(ctx, storeID, userID)
	assert.Equal(t, int64(600), avail)

	// Idempotent retry of deb-2
	err = svc.Debit(ctx, storeID, userID, 400, "GBP", "chargeback adjustment", "deb-2", &actor)
	require.NoError(t, err)
	avail, _, _, _ = svc.Balance(ctx, storeID, userID)
	assert.Equal(t, int64(600), avail)
}

func TestHoldCaptureReleaseLifecycle(t *testing.T) {
	db := setupTestDB(t)
	svc := credit.NewService(db)
	ctx := context.Background()

	storeID := uuid.New()
	userID := uuid.New()
	orderID := uuid.New()

	// Initial grant: 1000
	require.NoError(t, svc.Grant(ctx, storeID, userID, 1000, "GBP", "initial", "g-1", nil, nil))

	// 1. Hold 600 during checkout
	err := svc.Hold(ctx, storeID, userID, 600, "GBP", "hold-ord-1", &orderID)
	require.NoError(t, err)

	avail, held, _, _ := svc.Balance(ctx, storeID, userID)
	assert.Equal(t, int64(400), avail, "available must decrease by held amount")
	assert.Equal(t, int64(600), held, "held must increase by held amount")

	// Idempotent hold retry
	require.NoError(t, svc.Hold(ctx, storeID, userID, 600, "GBP", "hold-ord-1", &orderID))
	avail, held, _, _ = svc.Balance(ctx, storeID, userID)
	assert.Equal(t, int64(400), avail)
	assert.Equal(t, int64(600), held)

	// Hold exceeding available balance
	err = svc.Hold(ctx, storeID, userID, 500, "GBP", "hold-ord-2", &orderID)
	assert.ErrorIs(t, err, credit.ErrInsufficientCredit)

	// 2. Partial capture: order paid, capture 400 of the 600
	err = svc.Capture(ctx, storeID, userID, 400, "GBP", "cap-ord-1", &orderID)
	require.NoError(t, err)

	avail, held, _, _ = svc.Balance(ctx, storeID, userID)
	assert.Equal(t, int64(400), avail, "available remains 400")
	assert.Equal(t, int64(200), held, "held decreased from 600 to 200")

	// 3. Release remainder: 200 returned back to available
	err = svc.Release(ctx, storeID, userID, 200, "GBP", "rel-ord-1", &orderID)
	require.NoError(t, err)

	avail, held, _, _ = svc.Balance(ctx, storeID, userID)
	assert.Equal(t, int64(600), avail, "released 200 returned to available (400 + 200 = 600)")
	assert.Equal(t, int64(0), held, "held is back to 0")

	// Release more than held -> ErrInsufficientHold
	err = svc.Release(ctx, storeID, userID, 50, "GBP", "rel-extra", &orderID)
	assert.ErrorIs(t, err, credit.ErrInsufficientHold)
}

func TestAuditReconcile_MatchesLedger(t *testing.T) {
	db := setupTestDB(t)
	svc := credit.NewService(db)
	ctx := context.Background()

	storeID := uuid.New()
	userID := uuid.New()
	orderID := uuid.New()

	// Grant 1000
	require.NoError(t, svc.Grant(ctx, storeID, userID, 1000, "GBP", "seed", "idem-1", nil, nil))
	// Hold 400
	require.NoError(t, svc.Hold(ctx, storeID, userID, 400, "GBP", "idem-2", &orderID))
	// Capture 250
	require.NoError(t, svc.Capture(ctx, storeID, userID, 250, "GBP", "idem-3", &orderID))
	// Release 150
	require.NoError(t, svc.Release(ctx, storeID, userID, 150, "GBP", "idem-4", &orderID))
	// Debit 100
	require.NoError(t, svc.Debit(ctx, storeID, userID, 100, "GBP", "fee", "idem-5", nil))
	// Refund 50
	require.NoError(t, svc.Refund(ctx, storeID, userID, 50, "GBP", "return", "idem-6", nil, &orderID))

	// Reconcile:
	// Initial available: 1000 - 400 + 150 - 100 + 50 = 700
	// Initial held: 400 - 250 - 150 = 0
	expAvail, expHeld, actAvail, actHeld, matches, err := svc.AuditReconcile(ctx, storeID, userID)
	require.NoError(t, err)
	assert.True(t, matches, "cached balances must strictly match ledger sum")
	assert.Equal(t, int64(700), expAvail)
	assert.Equal(t, int64(700), actAvail)
	assert.Equal(t, int64(0), expHeld)
	assert.Equal(t, int64(0), actHeld)
}

func TestStoreScoping_Isolation(t *testing.T) {
	db := setupTestDB(t)
	svc := credit.NewService(db)
	ctx := context.Background()

	storeA := uuid.New()
	storeB := uuid.New()
	userID := uuid.New()

	// Grant 500 to storeA
	require.NoError(t, svc.Grant(ctx, storeA, userID, 500, "GBP", "storeA bonus", "idem-a", nil, nil))

	// StoreA should have 500, StoreB should have 0
	availA, _, _, _ := svc.Balance(ctx, storeA, userID)
	availB, _, _, _ := svc.Balance(ctx, storeB, userID)
	assert.Equal(t, int64(500), availA)
	assert.Equal(t, int64(0), availB)

	// Ledgers are strictly partitioned
	entriesA, _ := svc.Ledger(ctx, storeA, userID, 10)
	entriesB, _ := svc.Ledger(ctx, storeB, userID, 10)
	assert.Len(t, entriesA, 1)
	assert.Len(t, entriesB, 0)
}
