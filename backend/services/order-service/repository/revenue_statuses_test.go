package repositories

import (
	"slices"
	"testing"
)

func TestRevenueStatusesExcludeUnpaidAndCancelled(t *testing.T) {
	for _, s := range []string{"pending_payment", "pending", "payment_failed", "cancelled", "canceled", "refunded"} {
		if slices.Contains(RevenueStatuses, s) {
			t.Fatalf("%q must not count as revenue", s)
		}
	}
	if !slices.Contains(RevenueStatuses, "paid") {
		t.Fatal("paid orders must count as revenue")
	}
}
