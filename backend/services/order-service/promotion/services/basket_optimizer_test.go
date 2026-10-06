package services

import (
	"context"
	"testing"

	"order-service/promotion/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBasketOptimizer_EmptyCart(t *testing.T) {
	svc := NewBasketOptimizerService(zap.NewNop())
	ctx := context.Background()

	_, err := svc.Optimize(ctx, &models.OptimizeBasketRequest{
		Items:   []models.BasketItem{},
		Country: "US",
	})
	require.NotNil(t, err)
	assert.Equal(t, 400, err.StatusCode)
}

func TestBasketOptimizer_BelowFreeShippingThreshold(t *testing.T) {
	svc := NewBasketOptimizerService(zap.NewNop())
	ctx := context.Background()

	req := &models.OptimizeBasketRequest{
		Country: "US",
		Items: []models.BasketItem{
			{
				ProductID: "item-1",
				Name:      "Wireless Keyboard",
				Price:     55.00,
				Quantity:  1,
				WeightKg:  0.8,
			},
		},
	}

	resp, err := svc.Optimize(ctx, req)
	require.Nil(t, err)
	require.NotNil(t, resp)

	// Subtotal $55 is $20 below $75 free shipping
	assert.Equal(t, 55.00, resp.CurrentSubtotal)
	assert.Equal(t, 20.00, resp.AmountToFreeShipping)
	assert.True(t, resp.EligibleForDynamicBundle)
	assert.True(t, resp.CurrentShippingEstimate > 0)
	assert.NotEmpty(t, resp.Recommendations)

	// First recommendation should target the free shipping gap
	rec := resp.Recommendations[0]
	assert.True(t, rec.Price >= resp.AmountToFreeShipping)
	assert.True(t, rec.DiscountOffered > 0)
	assert.True(t, rec.NetSavings > 0)
}

func TestBasketOptimizer_AboveFreeShippingThreshold(t *testing.T) {
	svc := NewBasketOptimizerService(zap.NewNop())
	ctx := context.Background()

	req := &models.OptimizeBasketRequest{
		Country: "US",
		Items: []models.BasketItem{
			{
				ProductID: "item-premium",
				Name:      "Noise-Cancelling Headphones",
				Price:     150.00,
				Quantity:  1,
				WeightKg:  0.65,
			},
		},
	}

	resp, err := svc.Optimize(ctx, req)
	require.Nil(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, 150.00, resp.CurrentSubtotal)
	assert.Equal(t, 0.00, resp.AmountToFreeShipping)
	assert.Equal(t, 0.00, resp.CurrentShippingEstimate) // Free shipping achieved!
	assert.Equal(t, 0.35, resp.WeightHeadroomKg)        // 1.0 - 0.65 = 0.35kg
	assert.True(t, resp.EligibleForDynamicBundle)
	assert.NotEmpty(t, resp.Recommendations)
}

func TestBasketOptimizer_InternationalCountry(t *testing.T) {
	svc := NewBasketOptimizerService(zap.NewNop())
	ctx := context.Background()

	req := &models.OptimizeBasketRequest{
		Country: "GB",
		Items: []models.BasketItem{
			{
				ProductID: "item-uk",
				Name:      "Smart Speaker",
				Price:     40.00,
				Quantity:  1,
				WeightKg:  1.0,
			},
		},
	}

	resp, err := svc.Optimize(ctx, req)
	require.Nil(t, err)
	require.NotNil(t, resp)

	// International rate: base $25 + (1kg * $7.50) = $32.50
	assert.Equal(t, 32.50, resp.CurrentShippingEstimate)
}
