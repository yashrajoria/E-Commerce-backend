package providers

import (
	"fmt"
	"shipping-service/models"
	"strings"
)

// InternalDynamicProvider calculates shipping rates locally based on zones and weight.
type InternalDynamicProvider struct{}

func NewInternalDynamicProvider() *InternalDynamicProvider {
	return &InternalDynamicProvider{}
}

func (p *InternalDynamicProvider) GetRates(weightKg float64, destination models.Address) ([]models.ShippingRate, error) {
	if weightKg <= 0 {
		return nil, fmt.Errorf("invalid weight: %f", weightKg)
	}

	zone := p.determineZone(destination.Country)
	
	// Base rates and surcharges per zone
	var baseRate, weightSurcharge float64
	switch zone {
	case 1: // Domestic (US)
		baseRate = 5.00
		weightSurcharge = 1.50
	case 2: // North America (CA, MX)
		baseRate = 12.00
		weightSurcharge = 3.00
	default: // International
		baseRate = 25.00
		weightSurcharge = 7.50
	}

	serviceLevels := []struct {
		name       string
		multiplier float64
		days       int
	}{
		{"Standard", 1.0, 5},
		{"Express", 1.8, 2},
		{"Overnight", 3.2, 1},
	}

	rates := make([]models.ShippingRate, 0, len(serviceLevels))
	for _, sl := range serviceLevels {
		amount := (baseRate + (weightKg * weightSurcharge)) * sl.multiplier
		
		rates = append(rates, models.ShippingRate{
			Provider:      "ShopSwift Internal",
			ServiceLevel:  sl.name,
			Amount:        amount,
			Currency:      "USD",
			EstimatedDays: sl.days,
			RateID:        fmt.Sprintf("int-%d-%s", zone, strings.ToLower(sl.name)),
		})
	}

	return rates, nil
}

func (p *InternalDynamicProvider) determineZone(country string) int {
	country = strings.ToUpper(strings.TrimSpace(country))
	switch country {
	case "US":
		return 1
	case "CA", "MX":
		return 2
	default:
		return 3
	}
}
