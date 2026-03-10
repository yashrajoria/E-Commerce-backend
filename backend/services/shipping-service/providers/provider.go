package providers

import (
	"shipping-service/models"
)

// ShippingProvider defines the interface all carrier integrations must implement.
type ShippingProvider interface {
	// GetRates returns available shipping options for the given weight and destination.
	GetRates(weightKg float64, origin, destination models.Address) ([]models.ShippingRate, error)

	// CreateLabel purchases the selected rate and returns tracking + label info.
	CreateLabel(req models.CreateLabelRequest) (models.TrackingInfo, error)

	// TrackShipment returns the current tracking status for a given tracking code.
	TrackShipment(carrier, trackingCode string) (models.TrackingStatus, error)
}
