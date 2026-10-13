package controllers

import (
	"testing"

	"github.com/google/uuid"
)

func TestGenerateListCacheKeyIncludesStoreID(t *testing.T) {
	storeA := uuid.New()
	storeB := uuid.New()
	filtersA := &ProductFilters{StoreID: &storeA}
	filtersB := &ProductFilters{StoreID: &storeB}

	keyA := (&CacheManager{}).generateListCacheKey(1, 1, 20, filtersA)
	keyB := (&CacheManager{}).generateListCacheKey(1, 1, 20, filtersB)

	if keyA == keyB {
		t.Fatalf("expected different cache keys for different stores, got %q", keyA)
	}
}
