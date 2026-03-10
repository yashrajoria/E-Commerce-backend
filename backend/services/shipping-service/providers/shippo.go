package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"shipping-service/models"
	"time"
)

const shippoBaseURL = "https://api.goshippo.com"

// ShippoProvider implements ShippingProvider using the Shippo API.
type ShippoProvider struct {
	apiKey     string
	httpClient *http.Client
}

// NewShippoProvider creates a new ShippoProvider.
func NewShippoProvider(apiKey string) *ShippoProvider {
	return &ShippoProvider{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// ---- Shippo API request/response structs ----

type shippoAddress struct {
	Name    string `json:"name"`
	Street1 string `json:"street1"`
	Street2 string `json:"street2,omitempty"`
	City    string `json:"city"`
	State   string `json:"state"`
	Zip     string `json:"zip"`
	Country string `json:"country"`
	Phone   string `json:"phone,omitempty"`
	Email   string `json:"email,omitempty"`
}

type shippoParcel struct {
	Length       string `json:"length"`
	Width        string `json:"width"`
	Height       string `json:"height"`
	DistanceUnit string `json:"distance_unit"`
	Weight       string `json:"weight"`
	MassUnit     string `json:"mass_unit"`
}

type shippoShipmentRequest struct {
	AddressFrom shippoAddress `json:"address_from"`
	AddressTo   shippoAddress `json:"address_to"`
	Parcels     []shippoParcel `json:"parcels"`
	Async       bool          `json:"async"`
}

type shippoRate struct {
	ObjectID     string `json:"object_id"`
	Provider     string `json:"provider"`
	ServiceLevel struct {
		Name string `json:"name"`
	} `json:"servicelevel"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	EstimatedDays int    `json:"estimated_days"`
}

type shippoShipmentResponse struct {
	Rates []shippoRate `json:"rates"`
}

type shippoTransactionRequest struct {
	Rate  string `json:"rate"`
	Async bool   `json:"async"`
	LabelFileType string `json:"label_file_type"`
}

type shippoTransactionResponse struct {
	ObjectID     string `json:"object_id"`
	Status       string `json:"status"`
	TrackingNumber string `json:"tracking_number"`
	LabelURL     string `json:"label_url"`
	TrackingURLProvider string `json:"tracking_url_provider"`
	Messages     []struct {
		Text string `json:"text"`
	} `json:"messages"`
}

type shippoTrackResponse struct {
	TrackingNumber string `json:"tracking_number"`
	Carrier        string `json:"carrier"`
	TrackingStatus struct {
		Status    string `json:"status"`
		SubStatus string `json:"substatus"`
		Location  struct {
			City    string `json:"city"`
			State   string `json:"state"`
			Country string `json:"country"`
		} `json:"location"`
		StatusDate string `json:"status_date"`
	} `json:"tracking_status"`
}

// ---- ShippingProvider implementation ----

// GetRates creates a Shippo shipment and returns available rates.
func (s *ShippoProvider) GetRates(weightKg float64, origin, destination models.Address) ([]models.ShippingRate, error) {
	reqBody := shippoShipmentRequest{
		AddressFrom: toShippoAddress(origin),
		AddressTo:   toShippoAddress(destination),
		Parcels: []shippoParcel{
			{
				Length:       "10",
				Width:        "10",
				Height:       "10",
				DistanceUnit: "cm",
				Weight:       fmt.Sprintf("%.3f", weightKg),
				MassUnit:     "kg",
			},
		},
		Async: false,
	}

	var resp shippoShipmentResponse
	if err := s.doRequest(context.Background(), http.MethodPost, "/shipments/", reqBody, &resp); err != nil {
		return nil, fmt.Errorf("shippo GetRates: %w", err)
	}

	rates := make([]models.ShippingRate, 0, len(resp.Rates))
	for _, r := range resp.Rates {
		var amount float64
		fmt.Sscanf(r.Amount, "%f", &amount)
		rates = append(rates, models.ShippingRate{
			Provider:      r.Provider,
			ServiceLevel:  r.ServiceLevel.Name,
			Amount:        amount,
			Currency:      r.Currency,
			EstimatedDays: r.EstimatedDays,
			RateID:        r.ObjectID,
		})
	}

	return rates, nil
}

// CreateLabel purchases the selected Shippo rate and returns tracking info.
func (s *ShippoProvider) CreateLabel(req models.CreateLabelRequest) (models.TrackingInfo, error) {
	txReq := shippoTransactionRequest{
		Rate:          req.RateID,
		Async:         false,
		LabelFileType: "PDF",
	}

	var resp shippoTransactionResponse
	if err := s.doRequest(context.Background(), http.MethodPost, "/transactions/", txReq, &resp); err != nil {
		return models.TrackingInfo{}, fmt.Errorf("shippo CreateLabel: %w", err)
	}

	if resp.Status != "SUCCESS" {
		msg := "label creation failed"
		if len(resp.Messages) > 0 {
			msg = resp.Messages[0].Text
		}
		return models.TrackingInfo{}, fmt.Errorf("shippo CreateLabel: %s", msg)
	}

	return models.TrackingInfo{
		TrackingCode:   resp.TrackingNumber,
		LabelURL:       resp.LabelURL,
		TrackingURL:    resp.TrackingURLProvider,
		ShippoObjectID: resp.ObjectID,
	}, nil
}

// TrackShipment retrieves the current tracking status from Shippo.
func (s *ShippoProvider) TrackShipment(carrier, trackingCode string) (models.TrackingStatus, error) {
	path := fmt.Sprintf("/tracks/%s/%s", carrier, trackingCode)

	var resp shippoTrackResponse
	if err := s.doRequest(context.Background(), http.MethodGet, path, nil, &resp); err != nil {
		return models.TrackingStatus{}, fmt.Errorf("shippo TrackShipment: %w", err)
	}

	updatedAt := time.Now()
	if resp.TrackingStatus.StatusDate != "" {
		if t, err := time.Parse(time.RFC3339, resp.TrackingStatus.StatusDate); err == nil {
			updatedAt = t
		}
	}

	location := ""
	if l := resp.TrackingStatus.Location; l.City != "" {
		location = fmt.Sprintf("%s, %s, %s", l.City, l.State, l.Country)
	}

	return models.TrackingStatus{
		TrackingCode: resp.TrackingNumber,
		Status:       resp.TrackingStatus.Status,
		SubStatus:    resp.TrackingStatus.SubStatus,
		Location:     location,
		UpdatedAt:    updatedAt,
		Carrier:      resp.Carrier,
	}, nil
}

// ---- HTTP helper ----

func (s *ShippoProvider) doRequest(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, shippoBaseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "ShippoToken "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("shippo API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	if out != nil {
		if err := json.Unmarshal(respBytes, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// ---- Conversion helper ----

func toShippoAddress(a models.Address) shippoAddress {
	return shippoAddress{
		Name:    a.Name,
		Street1: a.Street1,
		Street2: a.Street2,
		City:    a.City,
		State:   a.State,
		Zip:     a.PostalCode,
		Country: a.Country,
		Phone:   a.Phone,
		Email:   a.Email,
	}
}
