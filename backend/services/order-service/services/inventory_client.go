package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// InventoryClient communicates with the inventory service via HTTP
type InventoryClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewInventoryClient creates a new InventoryClient
func NewInventoryClient(baseURL string) *InventoryClient {
	return &InventoryClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ReserveItem represents a single product + quantity for reservation
type ReserveItem struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// ReserveRequest is the payload sent to POST /inventory/reserve
type InventoryReserveRequest struct {
	OrderID string        `json:"order_id"`
	Items   []ReserveItem `json:"items"`
}

// ReleaseRequest is the payload sent to POST /inventory/release
type InventoryReleaseRequest struct {
	OrderID string        `json:"order_id"`
	Items   []ReserveItem `json:"items"`
}

// ConfirmRequest is the payload sent to POST /inventory/confirm
type InventoryConfirmRequest struct {
	OrderID string        `json:"order_id"`
	Items   []ReserveItem `json:"items"`
}

// StockCheckResult is the response from check/reserve
type StockCheckResult struct {
	ProductID    string `json:"product_id"`
	Available    int    `json:"available"`
	Reserved     int    `json:"reserved"`
	Requested    int    `json:"requested"`
	IsSufficient bool   `json:"is_sufficient"`
}

// InventoryInfo is the response from GET /inventory/:productId
type InventoryInfo struct {
	ProductID string    `json:"product_id"`
	Available int       `json:"available"`
	Reserved  int       `json:"reserved"`
	Threshold int       `json:"threshold"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetStock fetches inventory for a product
func (c *InventoryClient) GetStock(ctx context.Context, productID string) (*InventoryInfo, error) {
	url := fmt.Sprintf("%s/inventory/%s", c.baseURL, productID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("inventory service request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("inventory not found for product %s", productID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("inventory service returned %d", resp.StatusCode)
	}

	var info InventoryInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}

// ReserveStock reserves inventory for order items
func (c *InventoryClient) ReserveStock(ctx context.Context, orderID string, items []ReserveItem) error {
	payload := InventoryReserveRequest{
		OrderID: orderID,
		Items:   items,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/inventory/reserve", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("inventory reserve request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		json.NewDecoder(resp.Body).Decode(&errResp)
		errMsg := errResp["error"]
		if errMsg == "" {
			errMsg = fmt.Sprintf("inventory service returned %d", resp.StatusCode)
		}
		return fmt.Errorf("reserve failed: %s", errMsg)
	}

	log.Printf("[InventoryClient] Stock reserved for order=%s items=%d", orderID, len(items))
	return nil
}

// ReleaseStock releases reserved inventory (order cancelled/payment failed)
func (c *InventoryClient) ReleaseStock(ctx context.Context, orderID string, items []ReserveItem) error {
	payload := InventoryReleaseRequest{
		OrderID: orderID,
		Items:   items,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/inventory/release", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("inventory release request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("inventory release failed: status %d", resp.StatusCode)
	}

	log.Printf("[InventoryClient] Stock released for order=%s items=%d", orderID, len(items))
	return nil
}

// ConfirmStock confirms reserved inventory (payment succeeded)
func (c *InventoryClient) ConfirmStock(ctx context.Context, orderID string, items []ReserveItem) error {
	payload := InventoryConfirmRequest{
		OrderID: orderID,
		Items:   items,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/inventory/confirm", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("inventory confirm request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("inventory confirm failed: status %d", resp.StatusCode)
	}

	log.Printf("[InventoryClient] Stock confirmed for order=%s items=%d", orderID, len(items))
	return nil
}
