package db

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Inventory represents the stock details of a product
type Inventory struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`      // Unique identifier
	ProductID string             `bson:"product_id" json:"product_id"` // Reference to the product
	Quantity  int                `bson:"quantity" json:"quantity"`     // Available stock
	Reserved  int                `bson:"reserved" json:"reserved"`     // Reserved stock (for pending orders)
	Threshold int                `bson:"threshold" json:"threshold"`   // Minimum stock threshold for alerts
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"` // Last update timestamp
}

// InventoryUpdate is used for updating inventory quantity
type InventoryUpdate struct {
	Quantity int `json:"quantity"` // New stock quantity
}

// InventoryReservation is used when reserving stock for an order
type InventoryReservation struct {
	OrderID   string `json:"order_id"`   // Order reference
	ProductID string `json:"product_id"` // Product reference
	Quantity  int    `json:"quantity"`   // Quantity to reserve
}

// InventoryRelease is used for releasing reserved stock
type InventoryRelease struct {
	OrderID   string `json:"order_id"`   // Order reference
	ProductID string `json:"product_id"` // Product reference
}
