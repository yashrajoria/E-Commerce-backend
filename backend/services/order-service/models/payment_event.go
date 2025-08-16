package models

import "time"

// From cart-service → order-service
type CheckoutEvent struct {
	Event     string         `json:"event"`   // expected: "checkout.requested"
	UserID    string         `json:"user_id"` // must be UUID string
	Items     []CheckoutItem `json:"items"`
	Timestamp time.Time      `json:"timestamp"`
}

type CheckoutItem struct {
	ProductID string `json:"product_id"` // must be UUID string
	Quantity  int    `json:"quantity"`
}

// order-service → payment-service
type PaymentRequest struct {
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"`
	Amount  int    `json:"amount"` // minor units
}

// payment-service → order-service
type PaymentEvent struct {
	Type    string `json:"type"` // "payment_succeeded" | "payment_failed"
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"` // <-- Add this line

	PaymentID string    `json:"payment_id,omitempty"`
	Amount    int       `json:"amount,omitempty"`
	Currency  string    `json:"currency,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}
