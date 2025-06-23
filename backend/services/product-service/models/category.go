package models

import (
	"time"

	"github.com/google/uuid"
)

type Category struct {
	ID        uuid.UUID   `json:"_id" bson:"_id"`
	Name      string      `json:"name" bson:"name"`
	ParentIDs []uuid.UUID `json:"parent_ids,omitempty" bson:"parent_ids,omitempty"`
	Ancestors []uuid.UUID `json:"ancestors,omitempty" bson:"ancestors,omitempty"`
	Slug      string      `json:"slug" bson:"slug"`                       // You might want to slugify the name
	Path      []string    `json:"path,omitempty" bson:"path,omitempty"`   // e.g. ["Electronics", "Mobile", "Apple"]
	Level     int         `json:"level,omitempty" bson:"level,omitempty"` // Useful for sorting
	CreatedAt time.Time   `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time   `json:"updated_at" bson:"updated_at"`
}
