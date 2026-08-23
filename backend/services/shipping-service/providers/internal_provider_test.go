package providers_test

import (
	"errors"
	"shipping-service/models"
	"shipping-service/providers"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInternalDynamicProvider_GetRates_ValidCountry(t *testing.T) {
	p := providers.NewInternalDynamicProvider()

	rates, err := p.GetRates(2.0, models.Address{Country: "US"})
	assert.NoError(t, err)
	assert.NotEmpty(t, rates)
}

func TestInternalDynamicProvider_GetRates_UnknownCountryStillServiceable(t *testing.T) {
	p := providers.NewInternalDynamicProvider()

	// A well-formed but non-US/CA/MX code is a real international destination,
	// not an unserviceable one — it should still get rates.
	rates, err := p.GetRates(2.0, models.Address{Country: "JP"})
	assert.NoError(t, err)
	assert.NotEmpty(t, rates)
}

func TestInternalDynamicProvider_GetRates_InvalidCountryCode(t *testing.T) {
	p := providers.NewInternalDynamicProvider()

	cases := []string{"", "USA", "12", "??"}
	for _, c := range cases {
		_, err := p.GetRates(2.0, models.Address{Country: c})
		assert.Error(t, err, "country=%q", c)
		assert.True(t, errors.Is(err, providers.ErrUnserviceableDestination), "country=%q", c)
	}
}
