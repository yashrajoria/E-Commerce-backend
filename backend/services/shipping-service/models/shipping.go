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

// ShippingRate represents a single shipping option returned by the configured rate catalog.
type ShippingRate struct {
	Provider      string  `json:"provider"`      // e.g. "USPS", "FedEx"
	ServiceLevel  string  `json:"service_level"` // e.g. "Priority Mail"
	Amount        float64 `json:"amount"`        // in currency units, e.g. 9.99 USD
	Currency      string  `json:"currency"`      // e.g. "USD"
	EstimatedDays int     `json:"estimated_days"`
	RateID        string  `json:"rate_id"` // internal static rate rule ID
}

// ShippingRatesRequest is the payload for calculating shipping rates.
type ShippingRatesRequest struct {
	WeightKg    float64 `json:"weight_kg" binding:"required,gt=0"`
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
	ID                uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	OrderID           string    `gorm:"type:varchar(128);not null;index" json:"order_id"`
	UserID            string    `gorm:"type:varchar(128);not null;index" json:"user_id"`
	Carrier           string    `gorm:"type:varchar(64)" json:"carrier"`
	ServiceLevel      string    `gorm:"type:varchar(128)" json:"service_level"`
	TrackingCode      string    `gorm:"type:varchar(256);index" json:"tracking_code"`
	LabelURL          string    `gorm:"type:varchar(1024)" json:"label_url"`
	TrackingURL       string    `gorm:"type:varchar(1024)" json:"tracking_url"`
	ProviderReference string    `gorm:"type:varchar(256)" json:"provider_reference"`
	Status            string    `gorm:"type:varchar(32);not null;default:'pending'" json:"status"`
	WeightKg          float64   `gorm:"not null" json:"weight_kg"`
	// Origin/Destination stored as JSON strings for simplicity
	OriginJSON      string         `gorm:"type:jsonb" json:"-"`
	DestinationJSON string         `gorm:"type:jsonb" json:"-"`
	CreatedAt       time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}
