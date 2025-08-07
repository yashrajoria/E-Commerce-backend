package models

import "time"

type CartItem struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

type Cart struct {
	UserID    string     `json:"user_id"`
	Items     []CartItem `json:"items"`
	UpdatedAt time.Time  `json:"updated_at"`
}
