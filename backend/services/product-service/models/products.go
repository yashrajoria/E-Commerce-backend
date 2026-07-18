package models

import (
	"time"

	"github.com/google/uuid"
)

// Product is the API/domain model. Persistence is DynamoDB (see repository adapters).
type Product struct {
	ID           uuid.UUID   `json:"_id"`
	Name         string      `json:"name"`
	Price        float64     `json:"price"`
	Quantity     int         `json:"quantity"`
	Description  string      `json:"description,omitempty"`
	Images       []string    `json:"images,omitempty"`
	Brand        string      `json:"brand,omitempty"`
	SKU          string      `json:"sku"`
	CategoryIDs  []uuid.UUID `json:"category_ids,omitempty"`
	CategoryPath []string    `json:"category_path,omitempty"`
	IsFeatured   bool        `json:"is_featured"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
	DeletedAt    *time.Time  `json:"deleted_at,omitempty"`
}
