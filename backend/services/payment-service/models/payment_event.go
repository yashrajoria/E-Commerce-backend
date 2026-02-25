package models

import "time"

type PaymentEvent struct {
	Type        string    `json:"type"`     // e.g., "payment_succeeded" or "payment_failed"
	OrderID     string    `json:"order_id"` // UUID string from Order Service
	UserID      string    `json:"user_id"`  // <-- Add this line
	CheckoutURL string    `json:"checkout_url,omitempty"`
	Status      string    `json:"status"`     // "PROCESSING", "COMPLETED", "FAILED"
	PaymentID   string    `json:"payment_id"` // UUID from Payment Service DB
	Amount      int       `json:"amount"`     // smallest currency unit
	Currency    string    `json:"currency"`   // "usd", "inr"
	Timestamp   time.Time `json:"timestamp"`  // UTC event time
}

type PaymentRequest struct {
	OrderID        string `json:"order_id"`
	UserID         string `json:"user_id"`
	Amount         int    `json:"amount"`
	Currency       string `json:"currency"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}
