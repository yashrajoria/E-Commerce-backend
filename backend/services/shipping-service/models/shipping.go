package models

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
