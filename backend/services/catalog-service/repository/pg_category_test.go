package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"catalog-service/internal/pgtest"
	"catalog-service/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCategory(name string) *models.Category {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &models.Category{
		ID: uuid.New(), Name: name, Slug: strings.ToLower(name), Path: []string{name},
		IsActive: true, CreatedAt: now, UpdatedAt: now,
	}
}

func TestPGCategory_CreateFindUpdateDelete(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGCategoryRepo(db)
	ctx := context.Background()

	parent := newCategory("pgt-parent-" + uuid.NewString())
	child := newCategory("pgt-child-" + uuid.NewString())
	child.ParentIDs = []uuid.UUID{parent.ID}
	child.Ancestors = []uuid.UUID{parent.ID}
	child.Level = 1
	t.Cleanup(func() {
		db.Exec("DELETE FROM catalog.categories WHERE id IN ?", []string{parent.ID.String(), child.ID.String()})
	})
	require.NoError(t, r.Create(ctx, parent))
	require.NoError(t, r.Create(ctx, child))

	got, err := r.FindByID(ctx, child.ID)
	require.NoError(t, err)
	assert.Equal(t, child.Name, got.Name)
	assert.Equal(t, []uuid.UUID{parent.ID}, got.ParentIDs)
	assert.Equal(t, []uuid.UUID{parent.ID}, got.Ancestors)
	assert.Equal(t, []string{child.Name}, got.Path)
	assert.Equal(t, 1, got.Level)
	assert.True(t, got.IsActive)

	byName, err := r.FindByName(ctx, child.Name)
	require.NoError(t, err)
	assert.Equal(t, child.ID, byName.ID)

	many, err := r.FindByNames(ctx, []string{parent.Name, child.Name, child.Name, ""})
	require.NoError(t, err)
	assert.Len(t, many, 2)

	// Unknown keys (stored counts are computed on read) are ignored; known keys apply.
	require.NoError(t, r.Update(ctx, child.ID, map[string]interface{}{
		"name": child.Name + "-x", "direct_product_count": 3, "parent_ids": []uuid.UUID{},
	}))
	got, err = r.FindByID(ctx, child.ID)
	require.NoError(t, err)
	assert.Equal(t, child.Name+"-x", got.Name)
	assert.Empty(t, got.ParentIDs)
	assert.ErrorContains(t, r.Update(ctx, uuid.New(), map[string]interface{}{"name": "x"}), "not found")

	require.NoError(t, r.Delete(ctx, child.ID))
	_, err = r.FindByID(ctx, child.ID)
	assert.ErrorContains(t, err, "not found")
	all, err := r.FindAll(ctx)
	require.NoError(t, err)
	for _, c := range all {
		assert.NotEqual(t, child.ID, c.ID)
	}
	assert.ErrorContains(t, r.Delete(ctx, child.ID), "not found")
}

func TestPGCategory_HasProducts(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGCategoryRepo(db)
	ctx := context.Background()

	c := newCategory("pgt-has-" + uuid.NewString())
	t.Cleanup(func() { db.Exec("DELETE FROM catalog.categories WHERE id = ?", c.ID) })
	require.NoError(t, r.Create(ctx, c))
	pid := uuid.New()
	t.Cleanup(func() { db.Exec("DELETE FROM catalog.products WHERE id = ?", pid) }) // runs first (LIFO)

	has, err := r.HasProducts(ctx, c.ID)
	require.NoError(t, err)
	assert.False(t, has)

	require.NoError(t, db.Exec(`INSERT INTO catalog.products (id, name, sku, price) VALUES (?, 'p', ?, 1)`,
		pid, "PGT-"+pid.String()).Error)
	require.NoError(t, db.Exec(`INSERT INTO catalog.product_categories (category_id, product_id) VALUES (?, ?)`,
		c.ID, pid).Error)

	has, err = r.HasProducts(ctx, c.ID)
	require.NoError(t, err)
	assert.True(t, has)
}
