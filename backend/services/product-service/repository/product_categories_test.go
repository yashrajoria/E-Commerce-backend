package repository

import (
	"product-service/models"
	"testing"
)

func TestCategoryIDsFromFilter_AcceptsStringAndSlice(t *testing.T) {
	ids, ok := categoryIDsFromFilter(map[string]interface{}{"category_ids": "cat-1"})
	if !ok || len(ids) != 1 || ids[0] != "cat-1" {
		t.Fatalf("string form: got ok=%v ids=%v", ok, ids)
	}
	ids, ok = categoryIDsFromFilter(map[string]interface{}{"category_ids": []string{"a", "b"}})
	if !ok || len(ids) != 2 {
		t.Fatalf("slice form: got ok=%v ids=%v", ok, ids)
	}
	_, ok = categoryIDsFromFilter(map[string]interface{}{"brand": "x"})
	if ok {
		t.Fatal("expected no category ids")
	}
}

func TestProductMatchesResidualFilter(t *testing.T) {
	p := &models.Product{Brand: "Acme", Price: 10, Quantity: 2, IsFeatured: true}
	if !productMatchesResidualFilter(p, map[string]interface{}{"brand": "Acme", "is_featured": true}) {
		t.Fatal("expected match")
	}
	if productMatchesResidualFilter(p, map[string]interface{}{"brand": "Other"}) {
		t.Fatal("expected brand mismatch")
	}
	if productMatchesResidualFilter(p, map[string]interface{}{"in_stock": false}) {
		t.Fatal("expected in_stock mismatch")
	}
}
