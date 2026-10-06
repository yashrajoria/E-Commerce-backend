package models

import "time"

// ConfigureFlashSaleRequest configures or toggles the waiting room for a SKU.
type ConfigureFlashSaleRequest struct {
	ProductID       string `json:"product_id" binding:"required"`
	Active          bool   `json:"active"`
	TotalSlots      int    `json:"total_slots" binding:"gte=0"`
	LeaseTTLSeconds int    `json:"lease_ttl_seconds" binding:"gte=10"`
}

// EnterQueueRequest represents a customer attempting to access flash sale checkout.
type EnterQueueRequest struct {
	ProductID string `json:"product_id" binding:"required"`
	Quantity  int    `json:"quantity" binding:"required,min=1"`
	UserID    string `json:"user_id"`
}

// EnterQueueResponse tells the caller whether they got an instant lease or are waiting.
type EnterQueueResponse struct {
	Status               string     `json:"status"` // "GRANTED", "QUEUED", "INACTIVE", "SOLD_OUT"
	ProductID            string     `json:"product_id"`
	LeaseToken           string     `json:"lease_token,omitempty"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	Position             int64      `json:"position,omitempty"`
	EstimatedWaitSeconds int64      `json:"estimated_wait_seconds,omitempty"`
	TotalInQueue         int64      `json:"total_in_queue,omitempty"`
}

// QueueStatusResponse gives the customer's current queue rank or issued lease token.
type QueueStatusResponse struct {
	Status       string     `json:"status"` // "GRANTED", "QUEUED", "EXPIRED", "NOT_IN_QUEUE", "INACTIVE"
	ProductID    string     `json:"product_id"`
	LeaseToken   string     `json:"lease_token,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Position     int64      `json:"position,omitempty"`
	TotalInQueue int64      `json:"total_in_queue,omitempty"`
}

// ClaimLeaseRequest is sent when finalizing an order with an acquired lease token.
type ClaimLeaseRequest struct {
	ProductID  string `json:"product_id" binding:"required"`
	UserID     string `json:"user_id" binding:"required"`
	LeaseToken string `json:"lease_token" binding:"required"`
}

// ReleaseLeaseRequest frees an unconsumed lease if checkout is aborted or fails.
type ReleaseLeaseRequest struct {
	ProductID  string `json:"product_id" binding:"required"`
	UserID     string `json:"user_id" binding:"required"`
	LeaseToken string `json:"lease_token" binding:"required"`
	Quantity   int    `json:"quantity" binding:"required,min=1"`
}
