package models

import "time"

// PaymentEvent represents a payment outcome notification event published across the order lifecycle.
type PaymentEvent struct {
	Type              string    `json:"type"`     // e.g., "payment_succeeded" or "payment_failed"
	OrderID           string    `json:"order_id"` // UUID string from Order Service
	UserID            string    `json:"user_id"`  // <-- Add this line
	CheckoutURL       string    `json:"checkout_url,omitempty"`
	Status            string    `json:"status"`     // "PROCESSING", "COMPLETED", "FAILED"
	PaymentID         string    `json:"payment_id"` // UUID from Payment Service DB
	Amount            int       `json:"amount"`     // smallest currency unit
	Currency          string    `json:"currency"`   // ISO 4217 code, e.g. "usd"
	Timestamp         time.Time `json:"timestamp"`  // UTC event time
	CorrelationID     string    `json:"correlation_id,omitempty"`
	StoreID           string    `json:"store_id,omitempty"`
	StoreCreditAmount int64     `json:"store_credit_amount,omitempty"`
}

// ShippingAddress represents shipping address.
type ShippingAddress struct {
	FirstName  string `json:"first_name,omitempty"`
	LastName   string `json:"last_name,omitempty"`
	Line1      string `json:"line1,omitempty"`
	Line2      string `json:"line2,omitempty"`
	City       string `json:"city,omitempty"`
	State      string `json:"state,omitempty"`
	PostalCode string `json:"postal_code,omitempty"`
	Country    string `json:"country,omitempty"`
	Phone      string `json:"phone,omitempty"`
	Email      string `json:"email,omitempty"`
}

// PaymentRequest represents the internal payment execution payload transferred between order workers.
type PaymentRequest struct {
	EventID           string           `json:"event_id,omitempty"`
	OrderID           string           `json:"order_id"`
	UserID            string           `json:"user_id"`
	Email             string           `json:"email,omitempty"`
	ShippingAddress   *ShippingAddress `json:"shipping_address,omitempty"`
	Amount            int              `json:"amount"`
	Currency          string           `json:"currency"`
	IdempotencyKey    string           `json:"idempotency_key,omitempty"`
	CorrelationID     string           `json:"correlation_id,omitempty"`
	StoreID           string           `json:"store_id,omitempty"`
	StoreCreditAmount int64            `json:"store_credit_amount,omitempty"`
}
