package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Address represents a physical mailing address used for shipping.
type Address struct {
	Name       string `json:"name"`
	Street1    string `json:"street1"`
	Street2    string `json:"street2,omitempty"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"` // ISO 3166-1 alpha-2, e.g. "US"
	Phone      string `json:"phone,omitempty"`
	Email      string `json:"email,omitempty"`
}

// ShippingRate represents a single shipping option returned by a carrier.
type ShippingRate struct {
	Provider      string  `json:"provider"`      // e.g. "USPS", "FedEx"
	ServiceLevel  string  `json:"service_level"` // e.g. "Priority Mail"
	Amount        float64 `json:"amount"`        // in smallest currency unit (cents)
	Currency      string  `json:"currency"`      // e.g. "USD"
	EstimatedDays int     `json:"estimated_days"`
	RateID        string  `json:"rate_id"` // Shippo rate object ID, used to create label
}

// TrackingInfo is returned after a label is created successfully.
type TrackingInfo struct {
	TrackingCode   string `json:"tracking_code"`
	LabelURL       string `json:"label_url"`
	TrackingURL    string `json:"tracking_url"`
	Carrier        string `json:"carrier"`
	ShippoObjectID string `json:"shippo_object_id"`
}

// TrackingStatus represents the current status of a shipment.
type TrackingStatus struct {
	TrackingCode string    `json:"tracking_code"`
	Status       string    `json:"status"` // UNKNOWN, TRANSIT, DELIVERED, FAILURE, etc.
	SubStatus    string    `json:"sub_status,omitempty"`
	Location     string    `json:"location,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
	Carrier      string    `json:"carrier"`
}

// ShippingRatesRequest is the payload for calculating shipping rates.
type ShippingRatesRequest struct {
	WeightKg    float64 `json:"weight_kg" binding:"required,gt=0"`
	Destination Address `json:"destination" binding:"required"`
}

// CreateLabelRequest is the payload for creating a shipping label.
type CreateLabelRequest struct {
	OrderID     string  `json:"order_id" binding:"required"`
	UserID      string  `json:"user_id" binding:"required"`
	RateID      string  `json:"rate_id" binding:"required"` // selected from GetRates
	WeightKg    float64 `json:"weight_kg" binding:"required,gt=0"`
	Origin      Address `json:"origin" binding:"required"`
	Destination Address `json:"destination" binding:"required"`
}

// ShipmentStatus constants.
const (
	ShipmentStatusPending   = "pending"
	ShipmentStatusCreated   = "created"
	ShipmentStatusInTransit = "in_transit"
	ShipmentStatusDelivered = "delivered"
	ShipmentStatusFailed    = "failed"
)

// Shipment is the GORM model persisted in Postgres.
type Shipment struct {
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	OrderID        string    `gorm:"type:varchar(128);not null;index" json:"order_id"`
	UserID         string    `gorm:"type:varchar(128);not null;index" json:"user_id"`
	Carrier        string    `gorm:"type:varchar(64)" json:"carrier"`
	ServiceLevel   string    `gorm:"type:varchar(128)" json:"service_level"`
	TrackingCode   string    `gorm:"type:varchar(256);index" json:"tracking_code"`
	LabelURL       string    `gorm:"type:varchar(1024)" json:"label_url"`
	TrackingURL    string    `gorm:"type:varchar(1024)" json:"tracking_url"`
	ShippoObjectID string    `gorm:"type:varchar(256)" json:"shippo_object_id"`
	Status         string    `gorm:"type:varchar(32);not null;default:'pending'" json:"status"`
	WeightKg       float64   `gorm:"not null" json:"weight_kg"`
	// Origin/Destination stored as JSON strings for simplicity
	OriginJSON      string         `gorm:"type:jsonb" json:"-"`
	DestinationJSON string         `gorm:"type:jsonb" json:"-"`
	CreatedAt       time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// ShipmentCreatedEvent is published to SNS when a label is created.
type ShipmentCreatedEvent struct {
	EventType    string    `json:"event_type"`
	ShipmentID   string    `json:"shipment_id"`
	OrderID      string    `json:"order_id"`
	UserID       string    `json:"user_id"`
	TrackingCode string    `json:"tracking_code"`
	Carrier      string    `json:"carrier"`
	LabelURL     string    `json:"label_url"`
	Timestamp    time.Time `json:"timestamp"`
}

// ShipmentUpdatedEvent is published to SNS when tracking status changes.
type ShipmentUpdatedEvent struct {
	EventType    string    `json:"event_type"`
	ShipmentID   string    `json:"shipment_id"`
	OrderID      string    `json:"order_id"`
	TrackingCode string    `json:"tracking_code"`
	Status       string    `json:"status"`
	Timestamp    time.Time `json:"timestamp"`
}
