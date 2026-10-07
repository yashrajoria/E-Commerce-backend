package repository

import (
	"context"
	"fmt"
	"sort"
	"time"

	"catalog-service/inventory/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PGInventoryRepository implements InventoryRepository on Postgres (schema catalog).
// Reserve/release/confirm each run in one short transaction; products are always
// locked in id order so concurrent multi-product orders cannot deadlock.
type PGInventoryRepository struct{ db *gorm.DB }

var _ InventoryRepository = (*PGInventoryRepository)(nil)

func NewPGInventoryRepository(db *gorm.DB) *PGInventoryRepository {
	return &PGInventoryRepository{db: db}
}

type pgInventory struct {
	ProductID uuid.UUID
	Available int
	Reserved  int
	Threshold int
	UpdatedAt time.Time
}

const inventoryCols = "product_id, available, reserved, threshold, updated_at"

func (p pgInventory) model() *models.Inventory {
	return &models.Inventory{
		ProductID: p.ProductID.String(), Available: p.Available, Reserved: p.Reserved,
		Threshold: p.Threshold, UpdatedAt: p.UpdatedAt,
	}
}

// parseProductID maps a non-UUID id to ErrNotFound (any unknown string counts as missing).
func parseProductID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, ErrNotFound
	}
	return id, nil
}

type stockLine struct {
	id  uuid.UUID
	qty int
}

// mergeItems sums duplicate products and sorts by id, giving every transaction the same lock order.
func mergeItems(items []models.ReserveItem) ([]stockLine, error) {
	sum := make(map[uuid.UUID]int, len(items))
	for _, it := range items {
		id, err := uuid.Parse(it.ProductID)
		if err != nil {
			return nil, fmt.Errorf("invalid product_id %q", it.ProductID)
		}
		sum[id] += it.Quantity
	}
	lines := make([]stockLine, 0, len(sum))
	for id, q := range sum {
		lines = append(lines, stockLine{id, q})
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].id.String() < lines[j].id.String() })
	return lines, nil
}

func (r *PGInventoryRepository) Get(ctx context.Context, productID string) (*models.Inventory, error) {
	id, err := parseProductID(productID)
	if err != nil {
		return nil, err
	}
	var rows []pgInventory
	if err := r.db.WithContext(ctx).
		Raw("SELECT "+inventoryCols+" FROM catalog.inventory WHERE product_id = ?", id).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	inv := rows[0].model()

	var res []struct {
		OrderID  string
		Quantity int
	}
	if err := r.db.WithContext(ctx).
		Raw(`SELECT order_id, quantity FROM catalog.stock_reservations WHERE product_id = ? AND status = 'reserved'`, id).
		Scan(&res).Error; err != nil {
		return nil, err
	}
	if len(res) > 0 {
		inv.OrderReservations = make(map[string]int, len(res))
		for _, x := range res {
			inv.OrderReservations[x.OrderID] = x.Quantity
		}
	}
	return inv, nil
}

// BatchGet omits unknown ids and does not load order reservations.
func (r *PGInventoryRepository) BatchGet(ctx context.Context, productIDs []string) (map[string]*models.Inventory, error) {
	out := make(map[string]*models.Inventory, len(productIDs))
	var ids []string
	for _, s := range productIDs {
		if id, err := uuid.Parse(s); err == nil {
			ids = append(ids, id.String())
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []pgInventory
	if err := r.db.WithContext(ctx).
		Raw("SELECT "+inventoryCols+" FROM catalog.inventory WHERE product_id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ProductID.String()] = row.model()
	}
	return out, nil
}

func (r *PGInventoryRepository) AddStock(ctx context.Context, productID string, delta, threshold int) (*models.Inventory, error) {
	id, err := uuid.Parse(productID)
	if err != nil {
		return nil, fmt.Errorf("invalid product_id %q", productID)
	}
	var rows []pgInventory
	err = r.db.WithContext(ctx).Raw(`
		INSERT INTO catalog.inventory (product_id, available, threshold, updated_at)
		VALUES (?, ?, ?, now())
		ON CONFLICT (product_id) DO UPDATE
		SET available = catalog.inventory.available + EXCLUDED.available,
		    threshold = EXCLUDED.threshold,
		    updated_at = now()
		RETURNING `+inventoryCols, id, delta, threshold).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("upsert returned no row for product %s", productID)
	}
	return rows[0].model(), nil
}

// Update sets available and/or threshold; other keys are ignored.
func (r *PGInventoryRepository) Update(ctx context.Context, productID string, updates map[string]interface{}) error {
	id, err := parseProductID(productID)
	if err != nil {
		return err
	}
	set := map[string]interface{}{}
	for _, k := range []string{"available", "threshold"} {
		if v, ok := updates[k]; ok {
			set[k] = v
		}
	}
	if len(set) == 0 {
		return nil
	}
	set["updated_at"] = gorm.Expr("now()")
	res := r.db.WithContext(ctx).Table("catalog.inventory").Where("product_id = ?", id).Updates(set)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ReserveAll reserves every line atomically. A retry of an already-applied reserve
// (same order, product, quantity) succeeds without changing stock.
func (r *PGInventoryRepository) ReserveAll(ctx context.Context, orderID string, items []models.ReserveItem) error {
	lines, err := mergeItems(items)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, l := range lines {
			// Only inserts when the product has an inventory row; a conflict means this order already reserved it.
			res := tx.Exec(`
				INSERT INTO catalog.stock_reservations (order_id, product_id, quantity)
				SELECT ?, product_id, ? FROM catalog.inventory WHERE product_id = ?
				ON CONFLICT (order_id, product_id) DO NOTHING`, orderID, l.qty, l.id)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				var prev []struct {
					Status   string
					Quantity int
				}
				if err := tx.Raw(`SELECT status, quantity FROM catalog.stock_reservations WHERE order_id = ? AND product_id = ?`,
					orderID, l.id).Scan(&prev).Error; err != nil {
					return err
				}
				switch {
				case len(prev) == 0:
					return fmt.Errorf("%w: product=%s has no inventory", ErrInsufficientStock, l.id)
				case prev[0].Status == "reserved" && prev[0].Quantity == l.qty:
					continue // retry of a reserve that already succeeded
				default:
					return fmt.Errorf("%w: order=%s product=%s", ErrDuplicateReservation, orderID, l.id)
				}
			}
			res = tx.Exec(`
				UPDATE catalog.inventory
				SET available = available - ?, reserved = reserved + ?, updated_at = now()
				WHERE product_id = ? AND available >= ?`, l.qty, l.qty, l.id, l.qty)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return fmt.Errorf("%w: product=%s", ErrInsufficientStock, l.id)
			}
		}
		return nil
	})
}

// ReleaseAll returns reserved units to available.
func (r *PGInventoryRepository) ReleaseAll(ctx context.Context, orderID string, items []models.ReserveItem) error {
	return r.settle(ctx, orderID, items, "released", true)
}

// ConfirmAll makes reserved units permanent (they stay out of available, leave reserved).
func (r *PGInventoryRepository) ConfirmAll(ctx context.Context, orderID string, items []models.ReserveItem) error {
	return r.settle(ctx, orderID, items, "confirmed", false)
}

// settle moves each reservation out of 'reserved' using the STORED quantity. Replaying
// the same transition is a no-op; any other state is ErrNoReservation.
func (r *PGInventoryRepository) settle(ctx context.Context, orderID string, items []models.ReserveItem, to string, restock bool) error {
	lines, err := mergeItems(items)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, l := range lines {
			var done []struct{ Quantity int }
			if err := tx.Raw(`
				UPDATE catalog.stock_reservations SET status = ?, updated_at = now()
				WHERE order_id = ? AND product_id = ? AND status = 'reserved'
				RETURNING quantity`, to, orderID, l.id).Scan(&done).Error; err != nil {
				return err
			}
			if len(done) == 0 {
				var cur []struct{ Status string }
				if err := tx.Raw(`SELECT status FROM catalog.stock_reservations WHERE order_id = ? AND product_id = ?`,
					orderID, l.id).Scan(&cur).Error; err != nil {
					return err
				}
				if len(cur) == 1 && cur[0].Status == to {
					continue // replay
				}
				return fmt.Errorf("%w: order=%s product=%s", ErrNoReservation, orderID, l.id)
			}
			q, back := done[0].Quantity, 0
			if restock {
				back = q
			}
			if err := tx.Exec(`
				UPDATE catalog.inventory
				SET available = available + ?, reserved = reserved - ?, updated_at = now()
				WHERE product_id = ?`, back, q, l.id).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// RestockAll adds confirmed quantities back to available (paid order cancelled).
// Idempotency is guarded upstream by the order's paid → cancelled transition.
func (r *PGInventoryRepository) RestockAll(ctx context.Context, orderID string, items []models.ReserveItem) error {
	lines, err := mergeItems(items)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, l := range lines {
			res := tx.Exec(`UPDATE catalog.inventory SET available = available + ?, updated_at = now() WHERE product_id = ?`, l.qty, l.id)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return fmt.Errorf("%w: product=%s", ErrNotFound, l.id)
			}
		}
		return nil
	})
}

func (r *PGInventoryRepository) CheckStock(ctx context.Context, productID string, quantity int) (*models.StockCheckResult, error) {
	m, err := r.BatchGet(ctx, []string{productID})
	if err != nil {
		return nil, err
	}
	res := &models.StockCheckResult{ProductID: productID, Requested: quantity}
	if inv, ok := m[productID]; ok {
		res.Available, res.Reserved, res.IsSufficient = inv.Available, inv.Reserved, inv.Available >= quantity
	}
	return res, nil
}

// ListAll pages by product_id. ponytail: OFFSET paging on an admin-only list;
// keyset on product_id if the table grows large.
func (r *PGInventoryRepository) ListAll(ctx context.Context, limit, offset int) ([]models.Inventory, error) {
	var rows []pgInventory
	if err := r.db.WithContext(ctx).
		Raw("SELECT "+inventoryCols+" FROM catalog.inventory ORDER BY product_id LIMIT ? OFFSET ?", limit, offset).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]models.Inventory, len(rows))
	for i, row := range rows {
		out[i] = *row.model()
	}
	return out, nil
}
