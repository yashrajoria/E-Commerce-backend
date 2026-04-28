package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type PromotionClient struct {
	baseURL    string
	httpClient *http.Client
}

type ValidateCouponRequest struct {
	Code      string  `json:"code"`
	UserID    string  `json:"user_id"`
	CartTotal float64 `json:"cart_total"`
}

type ValidateCouponResponse struct {
	Valid          bool    `json:"valid"`
	Code           string  `json:"code"`
	DiscountAmount float64 `json:"discount_amount"`
	Message        string  `json:"message"`
}

func NewPromotionClient(baseURL string) *PromotionClient {
	if baseURL == "" {
		baseURL = "http://promotion-service:8090"
	}
	return &PromotionClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *PromotionClient) ValidateCoupon(ctx context.Context, code, userID string, total float64) (*ValidateCouponResponse, error) {
	reqBody := ValidateCouponRequest{
		Code:      code,
		UserID:    userID,
		CartTotal: total,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/coupons/validate", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("promotion service returned status: %d", resp.StatusCode)
	}

	var result ValidateCouponResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	zap.L().Info("Coupon validated", 
		zap.String("code", code), 
		zap.Bool("valid", result.Valid), 
		zap.Float64("discount", result.DiscountAmount))

	return &result, nil
}
