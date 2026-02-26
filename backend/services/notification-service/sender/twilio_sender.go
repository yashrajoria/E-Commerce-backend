package sender

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type TwilioSender struct {
	accountSID string
	authToken  string
	fromNumber string
	httpClient *http.Client
}

func NewTwilioSender() (*TwilioSender, error) {
	sid := os.Getenv("TWILIO_ACCOUNT_SID")
	token := os.Getenv("TWILIO_AUTH_TOKEN")
	from := os.Getenv("TWILIO_FROM_NUMBER")

	if sid == "" {
		return nil, fmt.Errorf("TWILIO_ACCOUNT_SID not set")
	}
	if token == "" {
		return nil, fmt.Errorf("TWILIO_AUTH_TOKEN not set")
	}
	if from == "" {
		return nil, fmt.Errorf("TWILIO_FROM_NUMBER not set")
	}

	return &TwilioSender{
		accountSID: sid,
		authToken:  token,
		fromNumber: from,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (t *TwilioSender) SendSMS(ctx context.Context, to, msg string) (SendResult, error) {
	apiURL := fmt.Sprintf(
		"https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json",
		t.accountSID,
	)

	formData := url.Values{}
	formData.Set("To", to)
	formData.Set("From", t.fromNumber)
	formData.Set("Body", msg)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL,
		strings.NewReader(formData.Encode()))
	if err != nil {
		return SendResult{}, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(t.accountSID, t.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return SendResult{}, fmt.Errorf("twilio request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return SendResult{}, fmt.Errorf("twilio error %s: %s", resp.Status, string(respBody))
	}

	return SendResult{
		MessageID: fmt.Sprintf("twilio-%d", time.Now().UnixNano()),
		SentAt:    time.Now(),
	}, nil
}
