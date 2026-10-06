package services

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"catalog-service/inventory/models"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func getTestRedis(t *testing.T) *redis.Client {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		opts = &redis.Options{Addr: redisURL}
	}

	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Skipping Redis integration test: Redis not reachable at %s (%v)", redisURL, err)
		return nil
	}
	return client
}

func TestWaitingRoom_FullLifecycle(t *testing.T) {
	rdb := getTestRedis(t)
	defer rdb.Close()

	logger := zap.NewNop()
	svc := NewWaitingRoomService(rdb, logger)
	ctx := context.Background()

	testProduct := fmt.Sprintf("test-prod-%d", time.Now().UnixNano())

	// 1. Inactive flash sale returns INACTIVE
	enterResp, err := svc.EnterQueue(ctx, &models.EnterQueueRequest{
		ProductID: testProduct,
		Quantity:  1,
		UserID:    "user-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "INACTIVE", enterResp.Status)

	// 2. Configure flash sale with exactly 2 slots
	err = svc.Configure(ctx, &models.ConfigureFlashSaleRequest{
		ProductID:       testProduct,
		Active:          true,
		TotalSlots:      2,
		LeaseTTLSeconds: 60,
	})
	require.NoError(t, err)

	// 3. User 1 enters -> GRANTED immediate lease
	u1Resp, err := svc.EnterQueue(ctx, &models.EnterQueueRequest{
		ProductID: testProduct,
		Quantity:  1,
		UserID:    "user-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "GRANTED", u1Resp.Status)
	assert.NotEmpty(t, u1Resp.LeaseToken)
	assert.NotNil(t, u1Resp.ExpiresAt)

	// 4. User 1 re-enters -> Idempotent, returns SAME lease
	u1ReEntry, err := svc.EnterQueue(ctx, &models.EnterQueueRequest{
		ProductID: testProduct,
		Quantity:  1,
		UserID:    "user-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "GRANTED", u1ReEntry.Status)
	assert.Equal(t, u1Resp.LeaseToken, u1ReEntry.LeaseToken)

	// 5. User 2 enters -> GRANTED (2nd slot filled)
	u2Resp, err := svc.EnterQueue(ctx, &models.EnterQueueRequest{
		ProductID: testProduct,
		Quantity:  1,
		UserID:    "user-2",
	})
	require.NoError(t, err)
	assert.Equal(t, "GRANTED", u2Resp.Status)
	assert.NotEmpty(t, u2Resp.LeaseToken)

	// 6. User 3 enters -> Slots exhausted, QUEUED at position 1
	u3Resp, err := svc.EnterQueue(ctx, &models.EnterQueueRequest{
		ProductID: testProduct,
		Quantity:  1,
		UserID:    "user-3",
	})
	require.NoError(t, err)
	assert.Equal(t, "QUEUED", u3Resp.Status)
	assert.Equal(t, int64(1), u3Resp.Position)
	assert.Equal(t, int64(1), u3Resp.TotalInQueue)

	// 7. User 4 enters -> QUEUED at position 2
	u4Resp, err := svc.EnterQueue(ctx, &models.EnterQueueRequest{
		ProductID: testProduct,
		Quantity:  1,
		UserID:    "user-4",
	})
	require.NoError(t, err)
	assert.Equal(t, "QUEUED", u4Resp.Status)
	assert.Equal(t, int64(2), u4Resp.Position)
	assert.Equal(t, int64(2), u4Resp.TotalInQueue)

	// 8. User 3 polls status -> Still QUEUED
	statusResp, err := svc.GetStatus(ctx, testProduct, "user-3")
	require.NoError(t, err)
	assert.Equal(t, "QUEUED", statusResp.Status)
	assert.Equal(t, int64(1), statusResp.Position)

	// 9. User 1 releases lease -> Slot freed back to pool
	err = svc.ReleaseLease(ctx, &models.ReleaseLeaseRequest{
		ProductID:  testProduct,
		UserID:     "user-1",
		LeaseToken: u1Resp.LeaseToken,
		Quantity:   1,
	})
	require.NoError(t, err)

	// 10. User 3 polls status again -> PROMOTED to GRANTED!
	promotedResp, err := svc.GetStatus(ctx, testProduct, "user-3")
	require.NoError(t, err)
	assert.Equal(t, "GRANTED", promotedResp.Status)
	assert.NotEmpty(t, promotedResp.LeaseToken)

	// 11. User 3 claims lease during checkout -> Success
	claimed, err := svc.ClaimLease(ctx, &models.ClaimLeaseRequest{
		ProductID:  testProduct,
		UserID:     "user-3",
		LeaseToken: promotedResp.LeaseToken,
	})
	require.NoError(t, err)
	assert.True(t, claimed)

	// 12. Claiming again fails (token consumed)
	claimedAgain, err := svc.ClaimLease(ctx, &models.ClaimLeaseRequest{
		ProductID:  testProduct,
		UserID:     "user-3",
		LeaseToken: promotedResp.LeaseToken,
	})
	require.NoError(t, err)
	assert.False(t, claimedAgain)

	// Clean up
	_ = svc.Reset(ctx, testProduct)
}

func TestWaitingRoom_ConcurrentContention(t *testing.T) {
	rdb := getTestRedis(t)
	defer rdb.Close()

	logger := zap.NewNop()
	svc := NewWaitingRoomService(rdb, logger)
	ctx := context.Background()

	testProduct := fmt.Sprintf("test-prod-concurrency-%d", time.Now().UnixNano())
	const totalSlots = 5
	const numUsers = 50

	err := svc.Configure(ctx, &models.ConfigureFlashSaleRequest{
		ProductID:       testProduct,
		Active:          true,
		TotalSlots:      totalSlots,
		LeaseTTLSeconds: 60,
	})
	require.NoError(t, err)

	type result struct {
		userID string
		status string
		token  string
		err    error
	}

	results := make(chan result, numUsers)

	// Fire 50 concurrent requests simultaneously
	for i := 1; i <= numUsers; i++ {
		uID := fmt.Sprintf("concurrent-user-%d", i)
		go func(userID string) {
			resp, err := svc.EnterQueue(ctx, &models.EnterQueueRequest{
				ProductID: testProduct,
				Quantity:  1,
				UserID:    userID,
			})
			if err != nil {
				results <- result{userID: userID, err: err}
				return
			}
			results <- result{userID: userID, status: resp.Status, token: resp.LeaseToken}
		}(uID)
	}

	grantedCount := 0
	queuedCount := 0

	for i := 1; i <= numUsers; i++ {
		res := <-results
		require.NoError(t, res.err)
		if res.status == "GRANTED" {
			grantedCount++
			assert.NotEmpty(t, res.token)
		} else if res.status == "QUEUED" {
			queuedCount++
		}
	}

	// Verify mathematically strict invariants:
	// Exactly totalSlots granted, exactly (numUsers - totalSlots) queued
	assert.Equal(t, totalSlots, grantedCount, "Exactly 5 slots must be granted under high contention")
	assert.Equal(t, numUsers-totalSlots, queuedCount, "Exactly 45 users must be cleanly queued")

	_ = svc.Reset(ctx, testProduct)
}

func TestWaitingRoom_SelfHealingExpiredLease(t *testing.T) {
	rdb := getTestRedis(t)
	defer rdb.Close()

	logger := zap.NewNop()
	svc := NewWaitingRoomService(rdb, logger)
	ctx := context.Background()

	testProduct := fmt.Sprintf("test-prod-expire-%d", time.Now().UnixNano())

	// Configure 1 slot with 1 second TTL
	err := svc.Configure(ctx, &models.ConfigureFlashSaleRequest{
		ProductID:       testProduct,
		Active:          true,
		TotalSlots:      1,
		LeaseTTLSeconds: 1,
	})
	require.NoError(t, err)

	// User 1 gets the 1 available slot
	u1Resp, err := svc.EnterQueue(ctx, &models.EnterQueueRequest{
		ProductID: testProduct,
		Quantity:  1,
		UserID:    "user-expire-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "GRANTED", u1Resp.Status)

	// User 2 enters immediately -> QUEUED
	u2Resp, err := svc.EnterQueue(ctx, &models.EnterQueueRequest{
		ProductID: testProduct,
		Quantity:  1,
		UserID:    "user-expire-2",
	})
	require.NoError(t, err)
	assert.Equal(t, "QUEUED", u2Resp.Status)

	// Wait 1.1s for user 1's lease to expire
	time.Sleep(1100 * time.Millisecond)

	// User 2 polls status -> Waiting room automatically reclaims User 1's expired lease and promotes User 2!
	u2Promoted, err := svc.GetStatus(ctx, testProduct, "user-expire-2")
	require.NoError(t, err)
	assert.Equal(t, "GRANTED", u2Promoted.Status, "User 2 must be promoted as expired lease self-heals")
	assert.NotEmpty(t, u2Promoted.LeaseToken)

	_ = svc.Reset(ctx, testProduct)
}

