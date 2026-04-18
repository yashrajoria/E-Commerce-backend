package providers

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"shipping-service/models"
	"slices"
	"sort"
	"strings"
)

//go:embed data/shipping_rates.json
var defaultShippingRatesData []byte

type staticRateCatalog struct {
	Rules []staticRateRule `json:"rules"`
}

type staticRateRule struct {
	RateID        string   `json:"rate_id"`
	Provider      string   `json:"provider"`
	ServiceLevel  string   `json:"service_level"`
	Countries     []string `json:"countries"`
	MinWeightKg   float64  `json:"min_weight_kg"`
	MaxWeightKg   float64  `json:"max_weight_kg"`
	Amount        float64  `json:"amount"`
	Currency      string   `json:"currency"`
	EstimatedDays int      `json:"estimated_days"`
}

// StaticRateProvider resolves rates from a local JSON catalog instead of a carrier API.
type StaticRateProvider struct {
	rules []staticRateRule
}

// NewStaticRateProvider loads shipping rules from an optional JSON file path.
// If no path is provided, the embedded default catalog is used.
func NewStaticRateProvider(path string) (*StaticRateProvider, error) {
	raw := defaultShippingRatesData
	if strings.TrimSpace(path) != "" {
		fileData, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read shipping rates file: %w", err)
		}
		raw = fileData
	}

	var catalog staticRateCatalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return nil, fmt.Errorf("decode shipping rates file: %w", err)
	}
	if len(catalog.Rules) == 0 {
		return nil, fmt.Errorf("shipping rates file contains no rules")
	}

	for i := range catalog.Rules {
		rule := &catalog.Rules[i]
		rule.Provider = strings.TrimSpace(rule.Provider)
		if rule.Provider == "" {
			rule.Provider = "Internal"
		}
		rule.ServiceLevel = strings.TrimSpace(rule.ServiceLevel)
		rule.Currency = strings.ToUpper(strings.TrimSpace(rule.Currency))
		for j := range rule.Countries {
			rule.Countries[j] = strings.ToUpper(strings.TrimSpace(rule.Countries[j]))
		}
		if err := validateRule(*rule); err != nil {
			return nil, fmt.Errorf("invalid shipping rate rule %q: %w", rule.RateID, err)
		}
	}

	return &StaticRateProvider{rules: catalog.Rules}, nil
}

// GetRates returns matching static rates for the given weight and destination.
func (p *StaticRateProvider) GetRates(weightKg float64, destination models.Address) ([]models.ShippingRate, error) {
	if weightKg <= 0 {
		return nil, fmt.Errorf("weight_kg must be greater than zero")
	}

	country := strings.ToUpper(strings.TrimSpace(destination.Country))
	if country == "" {
		return nil, fmt.Errorf("destination country is required")
	}

	exactMatches := make([]models.ShippingRate, 0)
	fallbackMatches := make([]models.ShippingRate, 0)

	for _, rule := range p.rules {
		if weightKg < rule.MinWeightKg {
			continue
		}
		if rule.MaxWeightKg > 0 && weightKg > rule.MaxWeightKg {
			continue
		}

		if slices.Contains(rule.Countries, country) {
			exactMatches = append(exactMatches, toShippingRate(rule))
			continue
		}
		if slices.Contains(rule.Countries, "*") {
			fallbackMatches = append(fallbackMatches, toShippingRate(rule))
		}
	}

	rates := exactMatches
	if len(rates) == 0 {
		rates = fallbackMatches
	}

	sort.SliceStable(rates, func(i, j int) bool {
		if rates[i].Amount != rates[j].Amount {
			return rates[i].Amount < rates[j].Amount
		}
		if rates[i].EstimatedDays != rates[j].EstimatedDays {
			return rates[i].EstimatedDays < rates[j].EstimatedDays
		}
		return rates[i].ServiceLevel < rates[j].ServiceLevel
	})

	return rates, nil
}

func validateRule(rule staticRateRule) error {
	if rule.RateID == "" {
		return fmt.Errorf("rate_id is required")
	}
	if len(rule.Countries) == 0 {
		return fmt.Errorf("countries is required")
	}
	if rule.ServiceLevel == "" {
		return fmt.Errorf("service_level is required")
	}
	if rule.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	if rule.Amount < 0 {
		return fmt.Errorf("amount must be non-negative")
	}
	if rule.MinWeightKg < 0 {
		return fmt.Errorf("min_weight_kg must be non-negative")
	}
	if rule.MaxWeightKg > 0 && rule.MaxWeightKg < rule.MinWeightKg {
		return fmt.Errorf("max_weight_kg must be greater than or equal to min_weight_kg")
	}
	return nil
}

func toShippingRate(rule staticRateRule) models.ShippingRate {
	return models.ShippingRate{
		Provider:      rule.Provider,
		ServiceLevel:  rule.ServiceLevel,
		Amount:        rule.Amount,
		Currency:      rule.Currency,
		EstimatedDays: rule.EstimatedDays,
		RateID:        rule.RateID,
	}
}
