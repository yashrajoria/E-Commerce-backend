package providers_test

import (
	"shipping-service/models"
	"shipping-service/providers"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticRateProvider_UsesExactCountryRules(t *testing.T) {
	provider, err := providers.NewStaticRateProvider("")
	require.NoError(t, err)

	rates, err := provider.GetRates(0.8, models.Address{Country: "US"})
	require.NoError(t, err)
	require.Len(t, rates, 2)
	assert.Equal(t, "us-standard-0-1", rates[0].RateID)
	assert.Equal(t, "us-express-0-1", rates[1].RateID)
}

func TestStaticRateProvider_FallsBackToWildcardRules(t *testing.T) {
	provider, err := providers.NewStaticRateProvider("")
	require.NoError(t, err)

	rates, err := provider.GetRates(2.0, models.Address{Country: "JP"})
	require.NoError(t, err)
	require.Len(t, rates, 2)
	assert.Equal(t, "row-standard-0-5", rates[0].RateID)
	assert.Equal(t, "row-express-0-5", rates[1].RateID)
}

func TestStaticRateProvider_RejectsMissingCountry(t *testing.T) {
	provider, err := providers.NewStaticRateProvider("")
	require.NoError(t, err)

	_, err = provider.GetRates(1.0, models.Address{})
	require.Error(t, err)
}
