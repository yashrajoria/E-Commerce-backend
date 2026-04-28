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

// InventoryClient calls the inventory-service HTTP API
type InventoryClient struct {
	baseURL string
	client  *http.Client
}

// NewInventoryClient creates a new InventoryClient
func NewInventoryClient(baseURL string) *InventoryClient {
	return &InventoryClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// SetStock initialises (or upserts) inventory for a product.
// Called after a product is created so the inventory table stays in sync.
func (ic *InventoryClient) SetStock(ctx context.Context, productID string, quantity int) error {
	body, _ := json.Marshal(map[string]interface{}{
		"product_id": productID,
		"available":  quantity,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ic.baseURL+"/inventory", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ic.client.Do(req)
	if err != nil {
		zap.L().Warn("inventory SetStock call failed", zap.String("product_id", productID), zap.Error(err))
		return fmt.Errorf("inventory request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("inventory SetStock returned status %d", resp.StatusCode)
	}
	return nil
}
