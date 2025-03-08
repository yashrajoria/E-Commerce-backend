package db

import "go.mongodb.org/mongo-driver/bson/primitive"

type Order struct {
	ID         primitive.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	line_items []string           `json:"line_items" bson:"line_items"`
	Name       string             `json:"name"`
	City       string             `json:"city"`
	Email      string             `json:"email"`
	PCode      string             `json:"pCode"`
	Address    string             `json:"address"`
	Phone      string             `json:"phone"`
	Paid       bool               `json:"paid"`
	Status     string             `json:"status"`
}
