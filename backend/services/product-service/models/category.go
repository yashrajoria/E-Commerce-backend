package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Category struct {
	ID        primitive.ObjectID   `json:"_id,omitempty" bson:"_id,omitempty"`
	Name      string               `json:"name" bson:"name"`
	ParentIDs []primitive.ObjectID `json:"parent_ids,omitempty" bson:"parent_ids,omitempty"`
	Ancestors []primitive.ObjectID `json:"ancestors,omitempty" bson:"ancestors,omitempty"`
	Slug      string               `json:"slug" bson:"slug"`                       // For SEO-friendly URLs
	Path      []string             `json:"path,omitempty" bson:"path,omitempty"`   // ["Mobile", "Android", "Samsung"]
	Level     int                  `json:"level,omitempty" bson:"level,omitempty"` // Depth in hierarchy, optional for sorting
	CreatedAt time.Time            `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time            `json:"updated_at" bson:"updated_at"`
}
