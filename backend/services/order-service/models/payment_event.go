package models

import "time"

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

// CheckoutEvent represents a cart checkout message emitted from catalog cart to order service.
type CheckoutEvent struct {
	Event             string           `json:"event"`   // expected: "checkout.requested"
	UserID            string           `json:"user_id"` // must be UUID string
	Email             string           `json:"email,omitempty"`
	ShippingAddress   *ShippingAddress `json:"shipping_address,omitempty"`
	IdempotencyKey    string           `json:"idempotency_key,omitempty"`
	Items             []CheckoutItem   `json:"items"`
	Timestamp         time.Time        `json:"timestamp"`
	OrderID           string           `json:"order_id"`
	StoreID           string           `json:"store_id,omitempty"`
	CouponCode        string           `json:"coupon_code,omitempty"`
	CorrelationID     string           `json:"correlation_id,omitempty"`
	StoreCreditAmount int64            `json:"store_credit_amount,omitempty"`
}

// CheckoutItem represents checkout item.
type CheckoutItem struct {
	ProductID string `json:"product_id"` // must be UUID string
	Quantity  int    `json:"quantity"`
}

// PaymentRequest represents the internal payment execution payload transferred between order workers.
type PaymentRequest struct {
	EventID           string           `json:"event_id,omitempty"`
	OrderID           string           `json:"order_id"`
	UserID            string           `json:"user_id"`
	Email             string           `json:"email,omitempty"`
	ShippingAddress   *ShippingAddress `json:"shipping_address,omitempty"`
	Amount            int              `json:"amount"` // minor units
	Currency          string           `json:"currency,omitempty"`
	StoreID           string           `json:"store_id,omitempty"`
	IdempotencyKey    string           `json:"idempotency_key,omitempty"`
	CorrelationID     string           `json:"correlation_id,omitempty"`
	StoreCreditAmount int64            `json:"store_credit_amount,omitempty"`
}

// PaymentEvent represents a payment outcome notification event published across the order lifecycle.
type PaymentEvent struct {
	Type              string    `json:"type"` // "payment_succeeded" | "payment_failed"
	OrderID           string    `json:"order_id"`
	UserID            string    `json:"user_id"` // <-- Add this line
	Email             string    `json:"email,omitempty"`
	StoreID           string    `json:"store_id,omitempty"`
	PaymentID         string    `json:"payment_id,omitempty"`
	Amount            int       `json:"amount,omitempty"`
	Currency          string    `json:"currency,omitempty"`
	Timestamp         time.Time `json:"timestamp,omitempty"`
	CorrelationID     string    `json:"correlation_id,omitempty"`
	StoreCreditAmount int64     `json:"store_credit_amount,omitempty"`
}

// StoreCreditEvent represents a domain event message for store credit event.
type StoreCreditEvent struct {
	EventID       string `json:"event_id"`
	EventType     string `json:"event_type"`
	UserID        string `json:"user_id"`
	StoreID       string `json:"store_id"`
	OrderID       string `json:"order_id,omitempty"`
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	CorrelationID string `json:"correlation_id,omitempty"`
}
