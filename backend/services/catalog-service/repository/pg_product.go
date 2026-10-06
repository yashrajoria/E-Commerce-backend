package repository

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"catalog-service/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PGProductRepo is the Postgres-backed ProductRepo (schema catalog).
type PGProductRepo struct{ db *gorm.DB }

var _ ProductRepo = (*PGProductRepo)(nil)

func NewPGProductRepo(db *gorm.DB) *PGProductRepo { return &PGProductRepo{db: db} }

type pgProduct struct {
	ID           uuid.UUID
	Name         string
	SKU          string `gorm:"column:sku"`
	Price        float64
	Quantity     int
	Description  string
	Brand        string
	Images       []byte
	CategoryPath []byte
	IsFeatured   bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
	CategoryIDs  []byte `gorm:"column:category_ids"`
}

// price is cast to float8 so the driver hands back a float64, not numeric text.
const productSelect = `
SELECT p.id, p.name, p.sku, p.price::float8 AS price, p.quantity, p.description, p.brand,
       p.images, p.category_path, p.is_featured, p.created_at, p.updated_at, p.deleted_at,
       COALESCE((SELECT jsonb_agg(pc.category_id) FROM catalog.product_categories pc
                 WHERE pc.product_id = p.id), '[]'::jsonb) AS category_ids
FROM catalog.products p`

func (p pgProduct) model() (*models.Product, error) {
	m := &models.Product{
		ID: p.ID, Name: p.Name, SKU: p.SKU, Price: p.Price, Quantity: p.Quantity,
		Description: p.Description, Brand: p.Brand, IsFeatured: p.IsFeatured,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, DeletedAt: p.DeletedAt,
	}
	for _, f := range []struct {
		src []byte
		dst interface{}
	}{{p.Images, &m.Images}, {p.CategoryPath, &m.CategoryPath}, {p.CategoryIDs, &m.CategoryIDs}} {
		if err := json.Unmarshal(f.src, f.dst); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// productWhere turns the service's filter map into a WHERE fragment ("TRUE" when
// empty). Fixed check order keeps the SQL deterministic.
func productWhere(filter map[string]interface{}) (string, []interface{}) {
	var conds []string
	var args []interface{}
	if v, ok := filter["is_featured"].(bool); ok {
		conds, args = append(conds, "p.is_featured = ?"), append(args, v)
	}
	if v, ok := filter["brand"].(string); ok && v != "" {
		conds, args = append(conds, "p.brand = ?"), append(args, v)
	}
	if v, ok := filter["min_price"]; ok {
		conds, args = append(conds, "p.price >= ?"), append(args, v)
	}
	if v, ok := filter["max_price"]; ok {
		conds, args = append(conds, "p.price <= ?"), append(args, v)
	}
	var cats []string
	switch v := filter["category_ids"].(type) {
	case []string:
		cats = v
	case string:
		if v != "" {
			cats = []string{v}
		}
	}
	if len(cats) > 0 {
		conds = append(conds, "EXISTS (SELECT 1 FROM catalog.product_categories pc WHERE pc.product_id = p.id AND pc.category_id IN ?)")
		args = append(args, cats)
	}
	if v, ok := filter["in_stock"].(bool); ok {
		if v {
			conds = append(conds, "p.quantity > 0")
		} else {
			conds = append(conds, "p.quantity <= 0")
		}
	}
	if len(conds) == 0 {
		return "TRUE", nil
	}
	return strings.Join(conds, " AND "), args
}

// query returns live products matching where, newest first. tail is " LIMIT ? OFFSET ?" etc.
func (r *PGProductRepo) query(ctx context.Context, where, tail string, args ...interface{}) ([]*models.Product, error) {
	var rows []pgProduct
	q := productSelect + " WHERE p.deleted_at IS NULL AND (" + where + ") ORDER BY p.created_at DESC, p.id" + tail
	if err := r.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*models.Product, 0, len(rows))
	for _, row := range rows {
		m, err := row.model()
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *PGProductRepo) FindByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	ps, err := r.query(ctx, "p.id = ?", "", id)
	if err != nil {
		return nil, err
	}
	if len(ps) == 0 {
		return nil, errRecordNotFound
	}
	return ps[0], nil
}

// Find pages with LIMIT/OFFSET. ponytail: offset paging; switch to keyset on
// (created_at, id) if deep pages get slow.
func (r *PGProductRepo) Find(ctx context.Context, filter map[string]interface{}, limit, skip int) ([]*models.Product, error) {
	where, args := productWhere(filter)
	tail := ""
	if limit > 0 {
		tail += " LIMIT ?"
		args = append(args, limit)
	}
	if skip > 0 {
		tail += " OFFSET ?"
		args = append(args, skip)
	}
	return r.query(ctx, where, tail, args...)
}

func (r *PGProductRepo) Count(ctx context.Context, filter map[string]interface{}) (int64, error) {
	where, args := productWhere(filter)
	var n int64
	err := r.db.WithContext(ctx).
		Raw("SELECT count(*) FROM catalog.products p WHERE p.deleted_at IS NULL AND ("+where+")", args...).
		Scan(&n).Error
	return n, err
}

func (r *PGProductRepo) Create(ctx context.Context, p *models.Product) error {
	return r.CreateMany(ctx, []models.Product{*p})
}

// CreateMany inserts products and their category links in one transaction.
func (r *PGProductRepo) CreateMany(ctx context.Context, ps []models.Product) error {
	if len(ps) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := insertProducts(tx, ps); err != nil {
			return err
		}
		var links [][2]uuid.UUID
		for _, p := range ps {
			for _, c := range p.CategoryIDs {
				links = append(links, [2]uuid.UUID{c, p.ID})
			}
		}
		return insertLinks(tx, links)
	})
}

// 12 bind params per product row; chunks stay far below Postgres' 65535 limit.
const productInsertChunk = 200

func insertProducts(tx *gorm.DB, ps []models.Product) error {
	for start := 0; start < len(ps); start += productInsertChunk {
		end := min(start+productInsertChunk, len(ps))
		var sb strings.Builder
		sb.WriteString(`INSERT INTO catalog.products
			(id, name, sku, price, quantity, description, brand, images, category_path, is_featured, created_at, updated_at)
			VALUES `)
		args := make([]interface{}, 0, (end-start)*12)
		for i, p := range ps[start:end] {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString("(?,?,?,?,?,?,?,?::jsonb,?::jsonb,?,?,?)")
			args = append(args, p.ID, p.Name, p.SKU, p.Price, p.Quantity, p.Description, p.Brand,
				jsonArray(p.Images), jsonArray(p.CategoryPath), p.IsFeatured, p.CreatedAt, p.UpdatedAt)
		}
		if err := tx.Exec(sb.String(), args...).Error; err != nil {
			return err
		}
	}
	return nil
}

// insertLinks takes [category, product] pairs.
func insertLinks(tx *gorm.DB, links [][2]uuid.UUID) error {
	const chunk = 500
	for start := 0; start < len(links); start += chunk {
		end := min(start+chunk, len(links))
		var sb strings.Builder
		sb.WriteString("INSERT INTO catalog.product_categories (category_id, product_id) VALUES ")
		args := make([]interface{}, 0, (end-start)*2)
		for i, l := range links[start:end] {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString("(?,?)")
			args = append(args, l[0], l[1])
		}
		sb.WriteString(" ON CONFLICT (category_id, product_id) DO NOTHING")
		if err := tx.Exec(sb.String(), args...).Error; err != nil {
			return err
		}
	}
	return nil
}

func replaceLinks(tx *gorm.DB, productID uuid.UUID, cats []uuid.UUID) error {
	if err := tx.Exec("DELETE FROM catalog.product_categories WHERE product_id = ?", productID).Error; err != nil {
		return err
	}
	links := make([][2]uuid.UUID, len(cats))
	for i, c := range cats {
		links[i] = [2]uuid.UUID{c, productID}
	}
	return insertLinks(tx, links)
}

func categoryIDsFromUpdate(updates map[string]interface{}) ([]uuid.UUID, bool) {
	switch v := updates["category_ids"].(type) {
	case []uuid.UUID:
		return v, true
	case []string:
		out := make([]uuid.UUID, 0, len(v))
		for _, s := range v {
			if u, err := uuid.Parse(s); err == nil {
				out = append(out, u)
			}
		}
		return out, true
	}
	return nil, false
}

// Update applies whitelisted columns; category_ids replaces the links atomically.
func (r *PGProductRepo) Update(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error {
	set := map[string]interface{}{}
	for k, v := range updates {
		switch k {
		case "name", "price", "quantity", "description", "brand", "sku", "is_featured":
			set[k] = v
		case "images", "category_path":
			set[k] = gorm.Expr("?::jsonb", jsonArray(v))
		}
	}
	cats, replace := categoryIDsFromUpdate(updates)
	if len(set) == 0 && !replace {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		set["updated_at"] = gorm.Expr("now()")
		res := tx.Table("catalog.products").Where("id = ? AND deleted_at IS NULL", id).Updates(set)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errRecordNotFound
		}
		if replace {
			return replaceLinks(tx, id, cats)
		}
		return nil
	})
}

func (r *PGProductRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Exec(`UPDATE catalog.products SET deleted_at = now(), updated_at = now() WHERE id = ? AND deleted_at IS NULL`, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errRecordNotFound
		}
		return tx.Exec("DELETE FROM catalog.product_categories WHERE product_id = ?", id).Error
	})
}

func (r *PGProductRepo) DeleteMany(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	strs := uuidStrings(ids)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE catalog.products SET deleted_at = now(), updated_at = now() WHERE id IN ? AND deleted_at IS NULL`, strs).Error; err != nil {
			return err
		}
		return tx.Exec("DELETE FROM catalog.product_categories WHERE product_id IN ?", strs).Error
	})
}

func (r *PGProductRepo) FindBySKUs(ctx context.Context, skus []string) ([]models.Product, error) {
	seen := make(map[string]struct{}, len(skus))
	uniq := make([]string, 0, len(skus))
	for _, s := range skus {
		if _, dup := seen[s]; s == "" || dup {
			continue
		}
		seen[s] = struct{}{}
		uniq = append(uniq, s)
	}
	if len(uniq) == 0 {
		return nil, nil
	}
	ps, err := r.query(ctx, "p.sku IN ?", "", uniq)
	if err != nil {
		return nil, err
	}
	out := make([]models.Product, len(ps))
	for i, p := range ps {
		out[i] = *p
	}
	return out, nil
}

// GetProductsByIDs skips soft-deleted products and ids that aren't UUIDs.
func (r *PGProductRepo) GetProductsByIDs(ctx context.Context, ids []string) ([]*models.Product, error) {
	valid := make([]string, 0, len(ids))
	for _, id := range ids {
		if u, err := uuid.Parse(id); err == nil {
			valid = append(valid, u.String())
		}
	}
	if len(valid) == 0 {
		return nil, nil
	}
	return r.query(ctx, "p.id IN ?", "", valid)
}
