package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Product struct {
	ID           primitive.ObjectID   `bson:"_id,omitempty" json:"_id"`
	Name         string               `json:"name"`
	Price        float64              `json:"price"`
	Quantity     int                  `json:"quantity"`
	Description  string               `json:"description"`
	Images       []string             `json:"images"`
	CategoryID   primitive.ObjectID   `bson:"category_id" json:"category_id"`
	CategoryIDs  []primitive.ObjectID `bson:"category_ids,omitempty" json:"category_ids"`
	CategoryPath []string             `bson:"category_path,omitempty" json:"category_path"`
	CreatedAt    time.Time            `json:"createdAt" bson:"createdAt"`
	UpdatedAt    time.Time            `json:"updatedAt" bson:"updatedAt"`
}
