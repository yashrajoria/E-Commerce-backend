package models

import (
	"time"

	"github.com/google/uuid"
)

type Product struct {
	ID           uuid.UUID   `bson:"_id" json:"_id"`
	Name         string      `bson:"name" json:"name"`
	Price        float64     `bson:"price" json:"price"`
	Quantity     int         `bson:"quantity" json:"quantity"`
	Description  string      `bson:"description,omitempty" json:"description,omitempty"`
	Images       []string    `bson:"images,omitempty" json:"images,omitempty"`
	Brand        string      `bson:"brand,omitempty" json:"brand,omitempty"`
	SKU          string      `bson:"sku" json:"sku"`
	CategoryIDs  []uuid.UUID `bson:"category_ids,omitempty" json:"category_ids,omitempty"`
	CategoryPath []string    `bson:"category_path,omitempty" json:"category_path,omitempty"`
	IsFeatured   bool        `bson:"is_featured" json:"is_featured"`
	CreatedAt    time.Time   `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time   `bson:"updated_at" json:"updated_at"`
	DeletedAt    *time.Time  `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
}
