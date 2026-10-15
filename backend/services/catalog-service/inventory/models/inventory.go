// Package models defines database schemas for inventory stock, reservations, and waiting room users.
package models

import (
	"time"
)

// Inventory represents real-time stock allocation and availability metrics for a product.
// It tracks on-hand available units, active order reservation holds, low-stock threshold triggers,
// and current order reservations.
// Persistence is backed by PostgreSQL in the catalog.inventory table.
//
// BERP Documentation:
//
// Behavior:
//   - Serves as the authoritative source of truth for product stock levels in the catalog service.
//   - Separates uncommitted inventory (Available) from committed checkout holds (Reserved).
//   - Enforces atomic stock deduction, reservation, and restocking via PostgreSQL row-level locks and transactions.
//   - Triggers low-stock alerts and supplier replenishment signals when Available falls below Threshold.
//   - Populates OrderReservations (map of order ID to quantity) when detailed product inventory is queried.
//
// Exceptions:
//   - ErrInsufficientStock when an order reservation exceeds available inventory (Available < requested).
//   - Non-negative check constraint violation if Available, Reserved, or Threshold is negative (< 0).
//   - Foreign key constraint failure if ProductID does not exist in catalog.products.
//   - Invalid UUID format error if ProductID cannot be parsed into a valid UUID.
//
// Returns / Relations:
//   - Database Table: "catalog.inventory" (PostgreSQL schema catalog).
//   - Primary / Foreign Key: ProductID references "catalog.products(id)".
//   - Has Many: Stock reservations via "catalog.stock_reservations" on ProductID.
//   - JSON Representation: Serialized with snake_case keys ("product_id", "available", "reserved", "threshold", etc.).
//
// Parameters / Properties:
//   - ProductID (string): Unique identifier (UUID string) of the associated catalog product.
//   - Available (int): Count of stock units currently available for immediate customer purchase.
//   - Reserved (int): Count of stock units currently committed to pending orders awaiting checkout payment.
//   - Threshold (int): Minimum stock quantity alerting threshold for reordering.
//   - OrderReservations (map[string]int): Mapping of order IDs to reserved quantities for active holds on this product.
//   - UpdatedAt (time.Time): Timestamp of the most recent stock update or reservation state change.
//
// Example:
//
//	inventory := models.Inventory{
//	    ProductID: "d0000000-0000-0000-0000-000000000001",
//	    Available: 150,
//	    Reserved:  10,
//	    Threshold: 20,
//	    OrderReservations: map[string]int{
//	        "ORD-20261015-87654": 2,
//	    },
//	    UpdatedAt: time.Now().UTC(),
//	}
type Inventory struct {
	ProductID         string         `json:"product_id"`
	Available         int            `json:"available"`
	Reserved          int            `json:"reserved"`
	Threshold         int            `json:"threshold"`
	OrderReservations map[string]int `json:"order_reservations,omitempty"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

// InventoryReservation represents an atomic stock reservation hold placed on a product for a pending customer order.
// It tracks reserved quantities, order association, and lifecycle states (reserved, confirmed, released).
// Persistence is backed by PostgreSQL in the catalog.stock_reservations table.
//
// BERP Documentation:
//
// Behavior:
//   - Represents a temporary stock allocation hold during customer checkout prior to payment finalization.
//   - Transitions through strict lifecycle statuses: 'reserved' -> 'confirmed' (payment succeeded) or 'released' (order cancelled/expired).
//   - Supports idempotent reservation re-attempts for the same order and product without double-reserving units.
//   - Completed and cancelled reservation records are automatically purged after 30 days by database maintenance jobs.
//
// Exceptions:
//   - Composite primary key violation if duplicate reservation attempts with conflicting quantities occur on (order_id, product_id).
//   - Quantity check constraint failure if Quantity is less than or equal to 0 (quantity > 0).
//   - Status constraint violation if Status is not one of 'reserved', 'confirmed', or 'released'.
//   - Foreign key constraint failure if ProductID does not exist in catalog.inventory.
//
// Returns / Relations:
//   - Database Table: "catalog.stock_reservations" (PostgreSQL schema catalog).
//   - Composite Primary Key: (OrderID, ProductID).
//   - Belongs To: Inventory via foreign key ProductID ("catalog.inventory.product_id").
//   - JSON Representation: Serialized with snake_case fields ("order_id", "product_id", "quantity", "status", etc.).
//
// Parameters / Properties:
//   - OrderID (string): Unique identifier of the purchasing customer order holding the reservation.
//   - ProductID (string): UUID string of the catalog product under hold.
//   - Quantity (int): Positive number of units reserved under this hold.
//   - Status (string): Reservation lifecycle status ('reserved', 'confirmed', 'released').
//   - CreatedAt (time.Time): Timestamp when the stock reservation hold was initially placed.
//   - UpdatedAt (time.Time): Timestamp when the reservation record was last modified or transitioned.
//
// Example:
//
//	reservation := models.InventoryReservation{
//	    OrderID:   "ORD-20261015-87654",
//	    ProductID: "d0000000-0000-0000-0000-000000000001",
//	    Quantity:  2,
//	    Status:    "reserved",
//	    CreatedAt: time.Now().UTC(),
//	    UpdatedAt: time.Now().UTC(),
//	}
type InventoryReservation struct {
	OrderID   string    `json:"order_id"`
	ProductID string    `json:"product_id"`
	Quantity  int       `json:"quantity"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
