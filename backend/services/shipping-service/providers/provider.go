package providers

import (
	"shipping-service/models"
)

// ShippingProvider defines the interface for shipping rate lookup.
type ShippingProvider interface {
	// GetRates returns available shipping options for the given destination and parcel weight.
	GetRates(weightKg float64, destination models.Address) ([]models.ShippingRate, error)
}
