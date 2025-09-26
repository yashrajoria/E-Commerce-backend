package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type Product struct {
	ID    uuid.UUID `json:"id"`
	Price float64   `json:"price"`
	Stock int       `json:"stock"`
}

func FetchProductByID(ctx context.Context, baseURL string, productID uuid.UUID) (*Product, error) {
	url := fmt.Sprintf("%s/products/internal/%s", baseURL, productID.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("product service returned %d", resp.StatusCode)
	}

	var prod Product
	if err := json.NewDecoder(resp.Body).Decode(&prod); err != nil {
		return nil, err
	}
	return &prod, nil
}
