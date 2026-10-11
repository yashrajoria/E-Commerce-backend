package services

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/yashrajoria/common/internalauth"
)

// CartClient clears a user's cart in catalog-service (mesh-authenticated).
type CartClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewCartClient(baseURL string) *CartClient {
	return &CartClient{baseURL: baseURL, httpClient: &http.Client{Timeout: 5 * time.Second}}
}

func (c *CartClient) ClearCart(ctx context.Context, userID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/cart/internal/%s", c.baseURL, url.PathEscape(userID)), nil)
	if err != nil {
		return err
	}
	internalauth.Apply(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("clear cart failed: status %d", resp.StatusCode)
	}
	return nil
}
