package models

import "time"

type CheckoutEvent struct {
	Event     string     `json:"event"` // e.g. "checkout.requested"
	UserID    string     `json:"user_id"`
	Items     []CartItem `json:"items"`
	Timestamp time.Time  `json:"timestamp"`
}
