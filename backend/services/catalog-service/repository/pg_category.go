package repository

import (
	"context"
	"encoding/json"
	"time"

	"catalog-service/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PGCategoryRepo is the Postgres-backed CategoryRepo (schema catalog).
type PGCategoryRepo struct{ db *gorm.DB }

var _ CategoryRepo = (*PGCategoryRepo)(nil)

func NewPGCategoryRepo(db *gorm.DB) *PGCategoryRepo { return &PGCategoryRepo{db: db} }

type pgCategory struct {
	ID        uuid.UUID
	Name      string
	Slug      string
	Image     string
	Level     int
	IsActive  bool
	ParentIDs []byte `gorm:"column:parent_ids"`
	Ancestors []byte
	Path      []byte
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

const categoryCols = `id, name, slug, image, level, is_active, parent_ids, ancestors, path, created_at, updated_at, deleted_at`

func (c pgCategory) model() (*models.Category, error) {
	m := &models.Category{
		ID: c.ID, Name: c.Name, Slug: c.Slug, Image: c.Image, Level: c.Level,
		IsActive: c.IsActive, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, DeletedAt: c.DeletedAt,
	}
	for _, f := range []struct {
		src []byte
		dst interface{}
	}{{c.ParentIDs, &m.ParentIDs}, {c.Ancestors, &m.Ancestors}, {c.Path, &m.Path}} {
		if err := json.Unmarshal(f.src, f.dst); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// find returns live categories matching where, ordered for stable output.
func (r *PGCategoryRepo) find(ctx context.Context, where string, args ...interface{}) ([]models.Category, error) {
	var rows []pgCategory
	q := "SELECT " + categoryCols + " FROM catalog.categories WHERE deleted_at IS NULL AND " + where + " ORDER BY level, name, id"
	if err := r.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]models.Category, 0, len(rows))
	for _, row := range rows {
		m, err := row.model()
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, nil
}

func (r *PGCategoryRepo) FindByID(ctx context.Context, id uuid.UUID) (*models.Category, error) {
	cats, err := r.find(ctx, "id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(cats) == 0 {
		return nil, errRecordNotFound
	}
	return &cats[0], nil
}

func (r *PGCategoryRepo) FindByName(ctx context.Context, name string) (*models.Category, error) {
	cats, err := r.find(ctx, "name = ?", name)
	if err != nil {
		return nil, err
	}
	if len(cats) == 0 {
		return nil, errRecordNotFound
	}
	return &cats[0], nil
}

func (r *PGCategoryRepo) FindByNames(ctx context.Context, names []string) ([]models.Category, error) {
	seen := make(map[string]struct{}, len(names))
	uniq := make([]string, 0, len(names))
	for _, n := range names {
		if _, dup := seen[n]; n == "" || dup {
			continue
		}
		seen[n] = struct{}{}
		uniq = append(uniq, n)
	}
	if len(uniq) == 0 {
		return []models.Category{}, nil
	}
	return r.find(ctx, "name IN ?", uniq)
}

func (r *PGCategoryRepo) FindAll(ctx context.Context) ([]models.Category, error) {
	return r.find(ctx, "TRUE")
}

func (r *PGCategoryRepo) Create(ctx context.Context, c *models.Category) error {
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO catalog.categories
		  (id, name, slug, image, level, is_active, parent_ids, ancestors, path, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?::jsonb, ?::jsonb, ?::jsonb, ?, ?)`,
		c.ID, c.Name, c.Slug, c.Image, c.Level, c.IsActive,
		jsonArray(c.ParentIDs), jsonArray(c.Ancestors), jsonArray(c.Path), c.CreatedAt, c.UpdatedAt,
	).Error
}

// Update applies the columns this repo owns; other keys (e.g. the stored product
// counts nothing reads back) are ignored.
func (r *PGCategoryRepo) Update(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error {
	set := map[string]interface{}{}
	for k, v := range updates {
		switch k {
		case "name", "slug", "image", "level", "is_active":
			set[k] = v
		case "parent_ids", "ancestors", "path":
			set[k] = gorm.Expr("?::jsonb", jsonArray(v))
		}
	}
	if len(set) == 0 {
		return nil
	}
	set["updated_at"] = gorm.Expr("now()")
	res := r.db.WithContext(ctx).Table("catalog.categories").
		Where("id = ? AND deleted_at IS NULL", id).Updates(set)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errRecordNotFound
	}
	return nil
}

func (r *PGCategoryRepo) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Exec(
		`UPDATE catalog.categories SET deleted_at = now(), updated_at = now() WHERE id = ? AND deleted_at IS NULL`, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errRecordNotFound
	}
	return nil
}

// HasProducts: product delete removes its links, so any link row is a live product.
func (r *PGCategoryRepo) HasProducts(ctx context.Context, categoryID uuid.UUID) (bool, error) {
	var ok bool
	err := r.db.WithContext(ctx).
		Raw(`SELECT EXISTS (SELECT 1 FROM catalog.product_categories WHERE category_id = ?)`, categoryID).
		Scan(&ok).Error
	return ok, err
}
