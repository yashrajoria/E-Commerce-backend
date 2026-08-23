package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/yashrajoria/common/internalauth"
)

// AuthClient talks to auth-service's internal-only endpoints.
type AuthClient struct {
	baseURL string
	client  *http.Client
}

func NewAuthClient() *AuthClient {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("AUTH_SERVICE_URL")), "/")
	if baseURL == "" {
		baseURL = "http://auth-service:8081"
	}
	return &AuthClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

// RevokeUserTokens asks auth-service to revoke every outstanding refresh
// token for a user, so a stolen session can't outlive a password change.
func (a *AuthClient) RevokeUserTokens(ctx context.Context, userID string) error {
	body, err := json.Marshal(map[string]string{"user_id": userID})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/auth/internal/revoke-tokens", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	internalauth.Apply(req)

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("auth-service revoke-tokens returned status %d", resp.StatusCode)
	}
	return nil
}
