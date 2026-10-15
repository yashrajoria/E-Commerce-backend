package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/yashrajoria/common/internalauth"
)

// Product represents product.
type Product struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Price float64   `json:"price"`
	Stock int       `json:"stock"`
}

// priceToCents converts the catalog's dollar price to the integer minor units
// (cents) used by orders, payments and Stripe.
func priceToCents(price float64) int {
	return int(math.Round(price * 100))
}

// ErrProductNotFound is permanent (product deleted); other fetch errors are transient.
var ErrProductNotFound = errors.New("product not found")

// FetchProductByID performs fetch product by id.
func FetchProductByID(ctx context.Context, baseURL string, productID uuid.UUID) (*Product, error) {
	url := fmt.Sprintf("%s/products/internal/%s", baseURL, productID.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	internalauth.Apply(req)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrProductNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("product service returned %d", resp.StatusCode)
	}

	var prod Product
	if err := json.NewDecoder(resp.Body).Decode(&prod); err != nil {
		return nil, err
	}
	return &prod, nil
}
