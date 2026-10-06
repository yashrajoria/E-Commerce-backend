package models

// BasketItem represents an individual line item in the customer's cart.
type BasketItem struct {
	ProductID string  `json:"product_id" binding:"required"`
	Name      string  `json:"name"`
	Price     float64 `json:"price" binding:"required,gte=0"`
	Quantity  int     `json:"quantity" binding:"required,min=1"`
	WeightKg  float64 `json:"weight_kg" binding:"gte=0"`
	Category  string  `json:"category"`
}

// OptimizeBasketRequest provides current cart contents and delivery country.
type OptimizeBasketRequest struct {
	Items    []BasketItem `json:"items" binding:"required,dive"`
	Country  string       `json:"country" binding:"required"`
	Subtotal float64      `json:"subtotal"`
	Currency string       `json:"currency"`
}

// UpsellRecommendation is an intelligent suggested addition that maximizes margin and shipping headroom.
type UpsellRecommendation struct {
	ProductID       string  `json:"product_id"`
	Title           string  `json:"title"`
	Price           float64 `json:"price"`
	WeightKg        float64 `json:"weight_kg"`
	DiscountOffered float64 `json:"discount_offered"`
	NetSavings      float64 `json:"net_savings"`
	Reason          string  `json:"reason"`
}

// OptimizeBasketResponse returns dynamic bundling incentives and shipping headroom analysis.
type OptimizeBasketResponse struct {
	CurrentWeightKg          float64                `json:"current_weight_kg"`
	CurrentSubtotal          float64                `json:"current_subtotal"`
	CurrentShippingEstimate  float64                `json:"current_shipping_estimate"`
	FreeShippingThreshold    float64                `json:"free_shipping_threshold"`
	AmountToFreeShipping     float64                `json:"amount_to_free_shipping"`
	WeightHeadroomKg         float64                `json:"weight_headroom_kg"`
	EligibleForDynamicBundle bool                   `json:"eligible_for_dynamic_bundle"`
	Recommendations          []UpsellRecommendation `json:"recommendations"`
}
