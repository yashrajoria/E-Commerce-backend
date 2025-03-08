package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type Category struct {
	ID       primitive.ObjectID  `json:"_id,omitempty" bson:"_id,omitempty"`
	Name     string              `json:"name" bson:"name"`
	ParentID *primitive.ObjectID `json:"parent_id,omitempty" bson:"parent_id,omitempty"` // Nullable for top-level categories
}
