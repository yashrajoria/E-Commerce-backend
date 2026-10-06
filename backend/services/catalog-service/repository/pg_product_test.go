package repository

import (
	"context"
	"testing"
	"time"

	"catalog-service/internal/pgtest"
	"catalog-service/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProductWhere(t *testing.T) {
	where, args := productWhere(nil)
	assert.Equal(t, "TRUE", where)
	assert.Empty(t, args)

	where, args = productWhere(map[string]interface{}{
		"is_featured": true, "brand": "Acme", "min_price": 5.0, "max_price": 9.0,
		"category_ids": []string{"c1", "c2"}, "in_stock": true,
	})
	assert.Equal(t, "p.is_featured = ? AND p.brand = ? AND p.price >= ? AND p.price <= ? AND "+
		"EXISTS (SELECT 1 FROM catalog.product_categories pc WHERE pc.product_id = p.id AND pc.category_id IN ?) "+
		"AND p.quantity > 0", where)
	assert.Equal(t, []interface{}{true, "Acme", 5.0, 9.0, []string{"c1", "c2"}}, args)

	where, args = productWhere(map[string]interface{}{"category_ids": "c1", "in_stock": false})
	assert.Contains(t, where, "pc.category_id IN ?")
	assert.Contains(t, where, "p.quantity <= 0")
	assert.Equal(t, []interface{}{[]string{"c1"}}, args)
}

func newProduct(brand string) models.Product {
	id := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	return models.Product{
		ID: id, Name: "pgt " + id.String()[:8], SKU: "PGT-" + id.String(), Price: 19.99, Quantity: 5,
		Brand: brand, Images: []string{"a.jpg", "b.jpg"}, CategoryPath: []string{"x"},
		CreatedAt: now, UpdatedAt: now,
	}
}

// makeCategory registers cleanup; call it BEFORE cleanupProducts so products are removed first (LIFO).
func makeCategory(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	c := newCategory("pgt-cat-" + uuid.NewString())
	require.NoError(t, NewPGCategoryRepo(db).Create(context.Background(), c))
	t.Cleanup(func() { db.Exec("DELETE FROM catalog.categories WHERE id = ?", c.ID) })
	return c.ID
}

func cleanupProducts(t *testing.T, db *gorm.DB, ids ...uuid.UUID) {
	t.Helper()
	t.Cleanup(func() { db.Exec("DELETE FROM catalog.products WHERE id IN ?", uuidStrings(ids)) })
}

func productIDs(ps []*models.Product) []uuid.UUID {
	out := make([]uuid.UUID, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}

func TestPGProduct_CreateAndFindByID(t *testing.T) {
	db := pgtest.DB(t)
	pr := NewPGProductRepo(db)
	ctx := context.Background()
	cat1, cat2 := makeCategory(t, db), makeCategory(t, db)

	p := newProduct("pgt-brand-" + uuid.NewString())
	p.CategoryIDs = []uuid.UUID{cat1, cat2}
	p.Description = "desc"
	cleanupProducts(t, db, p.ID)
	require.NoError(t, pr.Create(ctx, &p))

	got, err := pr.FindByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.SKU, got.SKU)
	assert.InDelta(t, 19.99, got.Price, 0.001)
	assert.Equal(t, 5, got.Quantity)
	assert.Equal(t, "desc", got.Description)
	assert.Equal(t, []string{"a.jpg", "b.jpg"}, got.Images)
	assert.Equal(t, []string{"x"}, got.CategoryPath)
	assert.ElementsMatch(t, []uuid.UUID{cat1, cat2}, got.CategoryIDs)
	assert.False(t, got.IsFeatured)
}

func TestPGProduct_FindFiltersPaginationCount(t *testing.T) {
	db := pgtest.DB(t)
	pr := NewPGProductRepo(db)
	ctx := context.Background()
	cat := makeCategory(t, db)
	brand := "pgt-brand-" + uuid.NewString()
	base := time.Now().UTC().Truncate(time.Microsecond)

	mk := func(price float64, qty int, featured bool, age time.Duration) models.Product {
		p := newProduct(brand)
		p.Price, p.Quantity, p.IsFeatured = price, qty, featured
		p.CreatedAt = base.Add(-age)
		p.CategoryIDs = []uuid.UUID{cat}
		return p
	}
	a := mk(10, 0, false, 3*time.Second) // oldest
	b := mk(20, 5, true, 2*time.Second)
	c := mk(30, 5, false, time.Second) // newest
	cleanupProducts(t, db, a.ID, b.ID, c.ID)
	require.NoError(t, pr.CreateMany(ctx, []models.Product{a, b, c}))

	find := func(filter map[string]interface{}, limit, skip int) []uuid.UUID {
		ps, err := pr.Find(ctx, filter, limit, skip)
		require.NoError(t, err)
		return productIDs(ps)
	}
	byBrand := map[string]interface{}{"brand": brand}

	assert.Equal(t, []uuid.UUID{c.ID, b.ID, a.ID}, find(byBrand, 0, 0), "newest first")
	assert.Equal(t, []uuid.UUID{c.ID, b.ID}, find(byBrand, 2, 0))
	assert.Equal(t, []uuid.UUID{a.ID}, find(byBrand, 2, 2))
	assert.Equal(t, []uuid.UUID{b.ID}, find(map[string]interface{}{"brand": brand, "is_featured": true}, 0, 0))
	assert.Equal(t, []uuid.UUID{b.ID}, find(map[string]interface{}{"brand": brand, "min_price": 15.0, "max_price": 25.0}, 0, 0))
	assert.Equal(t, []uuid.UUID{c.ID, b.ID}, find(map[string]interface{}{"brand": brand, "in_stock": true}, 0, 0))
	assert.Equal(t, []uuid.UUID{a.ID}, find(map[string]interface{}{"brand": brand, "in_stock": false}, 0, 0))
	assert.Equal(t, []uuid.UUID{c.ID, b.ID, a.ID}, find(map[string]interface{}{"category_ids": []string{cat.String()}}, 0, 0))
	assert.Equal(t, []uuid.UUID{c.ID, b.ID, a.ID}, find(map[string]interface{}{"category_ids": cat.String()}, 0, 0))

	n, err := pr.Count(ctx, byBrand)
	require.NoError(t, err)
	assert.EqualValues(t, 3, n)
	n, err = pr.Count(ctx, map[string]interface{}{"brand": brand, "in_stock": true})
	require.NoError(t, err)
	assert.EqualValues(t, 2, n)
}

func TestPGProduct_UpdateDeleteAndSKU(t *testing.T) {
	db := pgtest.DB(t)
	pr, cr := NewPGProductRepo(db), NewPGCategoryRepo(db)
	ctx := context.Background()
	cat1, cat2 := makeCategory(t, db), makeCategory(t, db)

	p := newProduct("pgt-brand-" + uuid.NewString())
	p.CategoryIDs = []uuid.UUID{cat1}
	dup := newProduct(p.Brand)
	dup.SKU = p.SKU
	cleanupProducts(t, db, p.ID, dup.ID)
	require.NoError(t, pr.Create(ctx, &p))

	// Update scalar + replace categories; unknown keys ignored; missing id errors.
	require.NoError(t, pr.Update(ctx, p.ID, map[string]interface{}{
		"price": 42.5, "category_ids": []uuid.UUID{cat2}, "images": []string{"z.jpg"}, "bogus": 1,
	}))
	got, err := pr.FindByID(ctx, p.ID)
	require.NoError(t, err)
	assert.InDelta(t, 42.5, got.Price, 0.001)
	assert.Equal(t, []uuid.UUID{cat2}, got.CategoryIDs)
	assert.Equal(t, []string{"z.jpg"}, got.Images)
	assert.ErrorContains(t, pr.Update(ctx, uuid.New(), map[string]interface{}{"price": 1.0}), "not found")

	// A live duplicate SKU is rejected by the database.
	assert.Error(t, pr.Create(ctx, &dup))

	found, err := pr.FindBySKUs(ctx, []string{p.SKU, "", p.SKU})
	require.NoError(t, err)
	assert.Len(t, found, 1)
	byIDs, err := pr.GetProductsByIDs(ctx, []string{p.ID.String(), "not-a-uuid"})
	require.NoError(t, err)
	assert.Len(t, byIDs, 1)

	// Soft delete hides it, drops its links, frees the SKU.
	require.NoError(t, pr.Delete(ctx, p.ID))
	_, err = pr.FindByID(ctx, p.ID)
	assert.ErrorContains(t, err, "not found")
	has, err := cr.HasProducts(ctx, cat2)
	require.NoError(t, err)
	assert.False(t, has)
	found, _ = pr.FindBySKUs(ctx, []string{p.SKU})
	assert.Empty(t, found)
	byIDs, _ = pr.GetProductsByIDs(ctx, []string{p.ID.String()})
	assert.Empty(t, byIDs)
	assert.ErrorContains(t, pr.Delete(ctx, p.ID), "not found")
	require.NoError(t, pr.Create(ctx, &dup), "SKU reusable after soft delete")
}

// 450 products x 2 categories crosses both batch boundaries: 200 products/insert
// and 500 links/insert.
func TestPGProduct_CreateManyAcrossChunks(t *testing.T) {
	db := pgtest.DB(t)
	pr := NewPGProductRepo(db)
	ctx := context.Background()
	cat1, cat2 := makeCategory(t, db), makeCategory(t, db)
	brand := "pgt-bulk-" + uuid.NewString()

	ps := make([]models.Product, 450)
	ids := make([]uuid.UUID, len(ps))
	for i := range ps {
		ps[i] = newProduct(brand)
		ps[i].CategoryIDs = []uuid.UUID{cat1, cat2}
		ids[i] = ps[i].ID
	}
	cleanupProducts(t, db, ids...)
	require.NoError(t, pr.CreateMany(ctx, ps))

	n, err := pr.Count(ctx, map[string]interface{}{"brand": brand})
	require.NoError(t, err)
	assert.EqualValues(t, 450, n)
	var links int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM catalog.product_categories WHERE category_id IN ?`,
		[]string{cat1.String(), cat2.String()}).Scan(&links).Error)
	assert.EqualValues(t, 900, links)

	// One bad row (duplicate SKU) must roll back the whole batch, links included.
	bad := []models.Product{newProduct(brand), ps[0]}
	bad[0].CategoryIDs = []uuid.UUID{cat1}
	cleanupProducts(t, db, bad[0].ID)
	assert.Error(t, pr.CreateMany(ctx, bad))
	_, err = pr.FindByID(ctx, bad[0].ID)
	assert.ErrorContains(t, err, "not found", "failed batch left nothing behind")
}

func TestPGProduct_DeleteMany(t *testing.T) {
	db := pgtest.DB(t)
	pr := NewPGProductRepo(db)
	ctx := context.Background()
	a, b := newProduct("pgt-"+uuid.NewString()), newProduct("pgt-"+uuid.NewString())
	cleanupProducts(t, db, a.ID, b.ID)
	require.NoError(t, pr.CreateMany(ctx, []models.Product{a, b}))

	require.NoError(t, pr.DeleteMany(ctx, []uuid.UUID{a.ID, b.ID, uuid.New()}), "unknown ids are ignored")
	for _, id := range []uuid.UUID{a.ID, b.ID} {
		_, err := pr.FindByID(ctx, id)
		assert.ErrorContains(t, err, "not found")
	}
	require.NoError(t, pr.DeleteMany(ctx, nil))
}
