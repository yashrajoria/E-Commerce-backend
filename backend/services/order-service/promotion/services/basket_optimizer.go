package services

import (
	"context"
	"math"
	"strings"

	"order-service/promotion/models"
	"go.uber.org/zap"
)

type BasketOptimizerService interface {
	Optimize(ctx context.Context, req *models.OptimizeBasketRequest) (*models.OptimizeBasketResponse, *ServiceError)
}

type basketOptimizerServiceImpl struct {
	logger *zap.Logger
}

func NewBasketOptimizerService(logger *zap.Logger) BasketOptimizerService {
	return &basketOptimizerServiceImpl{logger: logger}
}

func (s *basketOptimizerServiceImpl) Optimize(ctx context.Context, req *models.OptimizeBasketRequest) (*models.OptimizeBasketResponse, *ServiceError) {
	if len(req.Items) == 0 {
		return nil, &ServiceError{StatusCode: 400, Message: "Cart cannot be empty"}
	}

	var totalWeightKg float64
	var subtotal float64

	for _, item := range req.Items {
		itemWeight := item.WeightKg
		if itemWeight <= 0 {
			itemWeight = 0.35 // fallback estimate 350g per item
		}
		totalWeightKg += itemWeight * float64(item.Quantity)
		subtotal += item.Price * float64(item.Quantity)
	}

	if req.Subtotal > 0 {
		subtotal = req.Subtotal
	}

	// Calculate base shipping estimate
	country := strings.ToUpper(strings.TrimSpace(req.Country))
	if country == "" {
		country = "US"
	}

	var baseRate, weightSurcharge float64
	switch country {
	case "US":
		baseRate = 5.00
		weightSurcharge = 1.50
	case "CA", "MX":
		baseRate = 12.00
		weightSurcharge = 3.00
	default:
		baseRate = 25.00
		weightSurcharge = 7.50
	}

	const freeShippingThreshold = 75.00
	currentShipping := baseRate + (totalWeightKg * weightSurcharge)
	if subtotal >= freeShippingThreshold {
		currentShipping = 0.0
	}

	amountToFreeShipping := math.Max(0.0, freeShippingThreshold - subtotal)
	
	// Next weight step (1kg increments) headroom
	tierCeil := math.Ceil(totalWeightKg)
	if tierCeil == totalWeightKg {
		tierCeil += 1.0
	}
	weightHeadroom := math.Round((tierCeil - totalWeightKg) * 100) / 100

	recommendations := make([]models.UpsellRecommendation, 0)
	eligible := false

	if amountToFreeShipping > 0 && amountToFreeShipping <= 35.0 {
		eligible = true
		// High-margin micro-bundle recommendation: offsets shipping cost
		recPrice := math.Round((amountToFreeShipping + 4.00) * 100) / 100
		recDiscount := math.Min(5.00, math.Round(currentShipping * 0.7 * 100) / 100)
		netSavings := math.Round(currentShipping * 100) / 100

		recommendations = append(recommendations, models.UpsellRecommendation{
			ProductID:       "sku-acc-braided-cable",
			Title:           "Braided Ultra-Fast Cable",
			Price:           recPrice,
			WeightKg:        0.12,
			DiscountOffered: recDiscount,
			NetSavings:      netSavings,
			Reason:          "Unlocks free shipping threshold while staying well within parcel weight tier.",
		})

		recommendations = append(recommendations, models.UpsellRecommendation{
			ProductID:       "sku-acc-screen-cleaner",
			Title:           "Precision Screen Cleaner & Cloth",
			Price:           12.99,
			WeightKg:        0.08,
			DiscountOffered: 3.00,
			NetSavings:      netSavings,
			Reason:          "Ultralight accessory that bridges the free-shipping gap with 0 shipping penalty.",
		})
	} else if subtotal >= freeShippingThreshold && weightHeadroom >= 0.25 {
		// Consolidator incentive
		eligible = true
		recommendations = append(recommendations, models.UpsellRecommendation{
			ProductID:       "sku-acc-tech-organizer",
			Title:           "Travel Tech Pouch Organizer",
			Price:           19.99,
			WeightKg:        0.20,
			DiscountOffered: 4.00,
			NetSavings:      4.00,
			Reason:          "Leverages remaining weight capacity in your parcel with an instant $4 discount.",
		})
	}

	return &models.OptimizeBasketResponse{
		CurrentWeightKg:          math.Round(totalWeightKg*100) / 100,
		CurrentSubtotal:          math.Round(subtotal*100) / 100,
		CurrentShippingEstimate:  math.Round(currentShipping*100) / 100,
		FreeShippingThreshold:    freeShippingThreshold,
		AmountToFreeShipping:     math.Round(amountToFreeShipping*100) / 100,
		WeightHeadroomKg:         weightHeadroom,
		EligibleForDynamicBundle: eligible,
		Recommendations:          recommendations,
	}, nil
}
