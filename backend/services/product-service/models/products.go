package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type Product struct {
	ID       primitive.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	Name     string             `json:"title" bson:"title"`
	Price    float64            `json:"price" bson:"price"`
	Category primitive.ObjectID `json:"category" bson:"category"` // Store only category ID
	Images   []string           `json:"images" bson:"images"`
	Quantity int                `json:"quantity" bson:"quantity"`
}
