package services_test

import (
	"context"
	"sync"
	"testing"

	"promotion-service/models"
)

func TestIncrementCouponUsageHonorsAtomicUsageLimitUnderConcurrency(t *testing.T) {
	repo := &concurrentCouponRepo{mockRepo: &mockRepo{coupons: map[string]*models.Coupon{}}}
	coupon := activeCoupon("RACE", models.CouponTypePercentage, 10, 0, 1, 0)
	if err := repo.Create(context.Background(), coupon); err != nil {
		t.Fatal(err)
	}
	svc := newTestService(repo, &mockSNSPublisher{})

	var wg sync.WaitGroup
	var successes int
	var mu sync.Mutex
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := svc.IncrementCouponUsage(context.Background(), "RACE"); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if successes != 1 || coupon.UsedCount != 1 {
		t.Fatalf("usage limit was exceeded: successes=%d used_count=%d", successes, coupon.UsedCount)
	}
}
