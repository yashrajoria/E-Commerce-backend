// Package models defines data schemas for shipping quotes, addresses, and tracking updates.
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

// Shipment represents a physical carrier package dispatch fulfilling all or part of an order.
// It manages carrier integration attributes, tracking numbers, shipping labels, dispatch lifecycle states,
// and historical delivery checkpoints.
//
// BERP Documentation:
//
// Behavior:
//   - Encapsulates package fulfillment workflows connecting settled customer orders with logistics carriers (e.g. USPS, FedEx).
//   - Tracks state machine transitions across fulfillment: 'pending' -> 'created' -> 'in_transit' -> 'delivered' (or 'failed').
//   - Stores carrier tracking numbers, printable shipping label URLs, and carrier portal tracking links.
//   - Maintains serialized origin and destination snapshots in JSONB columns for immutable historical auditing.
//   - Implements soft deletion through GORM DeletedAt to retain logistics records for dispute resolution.
//
// Exceptions:
//   - Foreign key constraint error if OrderID does not correspond to an existing order.
//   - Foreign key constraint error if UserID does not correspond to a valid customer account.
//   - Validation error if WeightKg is less than or equal to zero.
//   - Carrier gateway exception if label purchase or tracking registration fails with carrier API.
//   - State machine conflict if attempting to modify a shipment marked in terminal 'delivered' status.
//
// Returns / Relations:
//   - Database Table: "shipments"
//   - Belongs To: Order via foreign key "OrderID".
//   - Belongs To: User via foreign key "UserID".
//   - JSON Representation: Serialized with snake_case keys (e.g. "order_id", "tracking_code", "label_url").
//
// Parameters / Properties:
//   - ID (uuid.UUID): Primary key, auto-generated via PostgreSQL gen_random_uuid().
//   - OrderID (uuid.UUID): Identifier of the parent order being fulfilled (indexed, not null).
//   - UserID (uuid.UUID): Identifier of customer receiving the package (indexed, not null).
//   - Carrier (string): Logistics provider name (e.g., "USPS", "FedEx", max 64 chars).
//   - ServiceLevel (string): Selected delivery service speed tier (e.g., "Priority Mail", "Ground", max 128 chars).
//   - TrackingCode (string): Carrier tracking number issued for package tracking (indexed, max 256 chars).
//   - LabelURL (string): Hosted URL of downloadable or printable shipping label (max 1024 chars).
//   - TrackingURL (string): Web URL pointing to public carrier package tracking interface (max 1024 chars).
//   - ShippoObjectID (string): External logistics provider transaction or rate object identifier (max 256 chars).
//   - Status (string): Current fulfillment state ('pending', 'created', 'in_transit', 'delivered', 'failed', defaults to 'pending').
//   - WeightKg (float64): Gross parcel weight measured in kilograms (must be > 0).
//   - OriginJSON (string): Serialized JSON snapshot of origin warehouse or store address.
//   - DestinationJSON (string): Serialized JSON snapshot of recipient shipping destination address.
//   - CreatedAt (time.Time): Record creation timestamp auto-populated by GORM.
//   - UpdatedAt (time.Time): Record modification timestamp auto-updated by GORM.
//   - DeletedAt (gorm.DeletedAt): Soft-delete timestamp for compliance and dispute retention.
//
// Example:
//
//	shipment := models.Shipment{
//	    OrderID:      uuid.MustParse("a0000000-0000-0000-0000-000000000001"),
//	    UserID:       uuid.MustParse("b0000000-0000-0000-0000-000000000002"),
//	    Carrier:      "USPS",
//	    ServiceLevel: "Priority Mail",
//	    TrackingCode: "9400111899562537625121",
//	    Status:       "created",
//	    WeightKg:     1.25,
//	}
type Shipment struct {
	ID              uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	OrderID         uuid.UUID      `gorm:"type:uuid;not null;index" json:"order_id"`
	UserID          uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	Carrier         string         `gorm:"type:varchar(64)" json:"carrier"`
	ServiceLevel    string         `gorm:"type:varchar(128)" json:"service_level"`
	TrackingCode    string         `gorm:"type:varchar(256);index" json:"tracking_code"`
	LabelURL        string         `gorm:"type:varchar(1024)" json:"label_url"`
	TrackingURL     string         `gorm:"type:varchar(1024)" json:"tracking_url"`
	ShippoObjectID  string         `gorm:"type:varchar(256)" json:"shippo_object_id"`
	Status          string         `gorm:"type:varchar(32);not null;default:'pending'" json:"status"`
	WeightKg        float64        `gorm:"not null" json:"weight_kg"`
	OriginJSON      string         `gorm:"type:jsonb" json:"-"`
	DestinationJSON string         `gorm:"type:jsonb" json:"-"`
	CreatedAt       time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// ShippingRate represents a single shipping option and delivery quote calculated for an order checkout.
// It details the carrier network, service tier speed, price amount, currency, and expected transit duration.
//
// BERP Documentation:
//
// Behavior:
//   - Generates available carrier delivery options evaluated against parcel weight and delivery destination.
//   - Offers shoppers comparative choices between economical ground shipping and accelerated express delivery.
//   - Supplies a unique RateID selected during checkout to bind rate estimates to completed orders.
//   - Acts as an in-memory calculation and transfer model returned by rate providers (e.g. Shippo or static zone tables).
//
// Exceptions:
//   - Quoted Amount must be a non-negative decimal value.
//   - EstimatedDays must be a non-negative integer representing business delivery days.
//   - Currency must adhere to ISO 4217 three-letter codes (e.g. "USD").
//   - RateID cannot be an empty string when selecting a shipping tier.
//
// Returns / Relations:
//   - Transient DTO: Returned in slices ([]ShippingRate) by ShippingProvider.GetRates and shipping controllers.
//   - JSON Representation: Serialized with snake_case keys (e.g. "service_level", "estimated_days", "rate_id").
//
// Parameters / Properties:
//   - Provider (string): Logistics provider name or carrier network (e.g., "USPS", "FedEx", "DHL").
//   - ServiceLevel (string): Human-readable delivery service speed name (e.g., "Priority Mail", "Standard Ground").
//   - Amount (float64): Quoted shipping cost in decimal currency units (e.g., 9.99 for $9.99 USD).
//   - Currency (string): ISO 4217 three-letter currency denomination code (e.g., "USD").
//   - EstimatedDays (int): Expected delivery duration in business transit days (e.g., 3).
//   - RateID (string): Unique identifier for this carrier rate rule or live quote token.
//
// Example:
//
//	rate := models.ShippingRate{
//	    Provider:      "USPS",
//	    ServiceLevel:  "Priority Mail",
//	    Amount:        9.99,
//	    Currency:      "USD",
//	    EstimatedDays: 2,
//	    RateID:        "rate-usps-priority",
//	}
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
