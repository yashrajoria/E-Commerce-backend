package models

import (
	"time"

	"github.com/google/uuid"
)

type Product struct {
	ID           uuid.UUID   `bson:"_id" json:"_id"`
	Name         string      `json:"name"`
	Price        float64     `json:"price"`
	Quantity     int         `json:"quantity"`
	Description  string      `json:"description"`
	Images       []string    `json:"images"`
	Brand        string      `json:"brand"`
	SKU          string      `json:"sku"`
	CategoryID   uuid.UUID   `bson:"category_id" json:"category_id"`
	CategoryIDs  []uuid.UUID `bson:"category_ids,omitempty" json:"category_ids"`
	CategoryPath []string    `bson:"category_path,omitempty" json:"category_path"`
	CreatedAt    time.Time   `json:"createdAt" bson:"createdAt"`
	UpdatedAt    time.Time   `json:"updatedAt" bson:"updatedAt"`
}
