package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type Category struct {
	ID        primitive.ObjectID   `json:"_id,omitempty" bson:"_id,omitempty"`
	Name      string               `json:"name" bson:"name"`
	ParentIDs []primitive.ObjectID `json:"parent_ids,omitempty" bson:"parent_ids,omitempty"`
	Ancestors []primitive.ObjectID `json:"ancestors,omitempty" bson:"ancestors,omitempty"`
}
