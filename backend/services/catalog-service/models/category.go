package models

import (
	"time"

	"github.com/google/uuid"
)

// Category is the API/domain model. Persistence is DynamoDB (see repository adapters).
type Category struct {
	ID                 uuid.UUID   `json:"_id"`
	Name               string      `json:"name"`
	ParentIDs          []uuid.UUID `json:"parent_ids,omitempty"`
	Image              string      `json:"image,omitempty"`
	Ancestors          []uuid.UUID `json:"ancestors,omitempty"`
	Slug               string      `json:"slug"`
	Path               []string    `json:"path,omitempty"`
	Level              int         `json:"level,omitempty"`
	IsActive           bool        `json:"is_active"`
	DirectProductCount int         `json:"directProductCount"` // Products directly in this category
	TotalProductCount  int         `json:"totalProductCount"`  // Including descendants
	CreatedAt          time.Time   `json:"created_at"`
	UpdatedAt          time.Time   `json:"updated_at"`
	DeletedAt          *time.Time  `json:"deleted_at,omitempty"`

	Children []*Category `json:"children,omitempty"` // transient field for frontend
}
