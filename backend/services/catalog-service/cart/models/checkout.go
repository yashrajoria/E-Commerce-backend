package models

import "time"

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

type CheckoutEvent struct {
	Event           string           `json:"event"` // e.g. "checkout.requested"
	UserID          string           `json:"user_id"`
	Email           string           `json:"email,omitempty"`
	ShippingAddress *ShippingAddress `json:"shipping_address,omitempty"`
	IdempotencyKey  string           `json:"idempotency_key,omitempty"`
	Items           []CartItem       `json:"items"`
	Timestamp       time.Time        `json:"timestamp"`
	OrderID         string           `json:"order_id"`
	CouponCode      string           `json:"coupon_code,omitempty"`
	CorrelationID   string           `json:"correlation_id,omitempty"`
}
