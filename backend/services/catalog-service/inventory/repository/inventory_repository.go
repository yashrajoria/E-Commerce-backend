package repository

import (
	"context"
	"errors"

	"catalog-service/inventory/models"
)

var (
	ErrNotFound             = errors.New("inventory record not found")
	ErrInsufficientStock    = errors.New("insufficient stock")
	ErrDuplicateReservation = errors.New("order already holds a different reservation for this product")
	ErrNoReservation        = errors.New("no active reservation for this order and product")
)

// InventoryRepository defines the interface for inventory data access.
type InventoryRepository interface {
	Get(ctx context.Context, productID string) (*models.Inventory, error)
	BatchGet(ctx context.Context, productIDs []string) (map[string]*models.Inventory, error)
	// AddStock upserts: adds delta to available and overwrites threshold.
	AddStock(ctx context.Context, productID string, delta, threshold int) (*models.Inventory, error)
	Update(ctx context.Context, productID string, updates map[string]interface{}) error
	ReserveAll(ctx context.Context, orderID string, items []models.ReserveItem) error
	ReleaseAll(ctx context.Context, orderID string, items []models.ReserveItem) error
	ConfirmAll(ctx context.Context, orderID string, items []models.ReserveItem) error
	RestockAll(ctx context.Context, orderID string, items []models.ReserveItem) error
	CheckStock(ctx context.Context, productID string, quantity int) (*models.StockCheckResult, error)
	ListAll(ctx context.Context, limit, offset int) ([]models.Inventory, error)
}
