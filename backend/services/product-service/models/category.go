package models

import (
	"time"

	"github.com/google/uuid"
)

type Category struct {
	ID        uuid.UUID   `bson:"_id" json:"_id"`
	Name      string      `bson:"name" json:"name"`
	ParentIDs []uuid.UUID `bson:"parent_ids,omitempty" json:"parent_ids,omitempty"`
	Image     string      `bson:"image,omitempty" json:"image,omitempty"`
	Ancestors []uuid.UUID `bson:"ancestors,omitempty" json:"ancestors,omitempty"`
	Slug      string      `bson:"slug" json:"slug"`
	Path      []string    `bson:"path,omitempty" json:"path,omitempty"`
	Level     int         `bson:"level,omitempty" json:"level,omitempty"`
	CreatedAt time.Time   `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time   `bson:"updated_at" json:"updated_at"`
	DeletedAt *time.Time  `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
}
