package models

import (
	"time"
)

// Inventory represents the stock details of a product.
type Inventory struct {
	ProductID         string         `json:"product_id"`
	Available         int            `json:"available"`
	Reserved          int            `json:"reserved"`
	Threshold         int            `json:"threshold"`
	OrderReservations map[string]int `json:"order_reservations,omitempty"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

// SetStockRequest is used to initialize or overwrite inventory for a product
type SetStockRequest struct {
	ProductID string `json:"product_id" binding:"required"`
	Available int    `json:"available" binding:"required,gte=0"`
	Threshold int    `json:"threshold" binding:"gte=0"`
}

// UpdateStockRequest is used to adjust the available quantity
type UpdateStockRequest struct {
	Available *int `json:"available" binding:"omitempty,gte=0"`
	Threshold *int `json:"threshold" binding:"omitempty,gte=0"`
}

// ReserveRequest is used when reserving stock for an order
type ReserveRequest struct {
	OrderID string        `json:"order_id" binding:"required"`
	Items   []ReserveItem `json:"items" binding:"required,dive"`
}

// ReserveItem is a single product + quantity to reserve
type ReserveItem struct {
	ProductID string `json:"product_id" binding:"required"`
	Quantity  int    `json:"quantity" binding:"required,min=1"`
}

// ReleaseRequest is used for releasing reserved stock (e.g. order cancelled)
type ReleaseRequest struct {
	OrderID string        `json:"order_id" binding:"required"`
	Items   []ReserveItem `json:"items" binding:"required,dive"`
}

// ConfirmRequest is used when payment succeeds — deducts reserved stock permanently
type ConfirmRequest struct {
	OrderID string        `json:"order_id" binding:"required"`
	Items   []ReserveItem `json:"items" binding:"required,dive"`
}

// RestockRequest is used when a paid (already confirmed) order is cancelled —
// adds the confirmed quantities back to available stock. Unlike ReleaseRequest
// there is no reservation left to remove since ConfirmStock already cleared it.
type RestockRequest struct {
	OrderID string        `json:"order_id" binding:"required"`
	Items   []ReserveItem `json:"items" binding:"required,dive"`
}

// StockCheckResult represents availability info for a single product
type StockCheckResult struct {
	ProductID    string `json:"product_id"`
	Available    int    `json:"available"`
	Reserved     int    `json:"reserved"`
	Requested    int    `json:"requested"`
	IsSufficient bool   `json:"is_sufficient"`
}
