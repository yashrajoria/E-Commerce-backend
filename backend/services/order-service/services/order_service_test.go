package services

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// mockSNS implements aws.SNSPublisher (avoids importing aws pkg in test)
type mockSNS struct {
	publishedArn string
	publishedMsg []byte
}

func (m *mockSNS) Publish(ctx context.Context, topicArn string, message []byte) error {
	m.publishedArn = topicArn
	m.publishedMsg = append([]byte(nil), message...)
	return nil
}

func TestCreateOrder_PublishesToSNS(t *testing.T) {
	// Arrange
	sns := &mockSNS{}

	// Use a nil repo (we don't reach DB in CreateOrder)
	svc := NewOrderServiceSQS(nil, sns, "arn:aws:sns:eu-west-2:000000000000:order-events")

	req := &CreateOrderRequest{
		Items: []struct {
			ProductID uuid.UUID "json:\"product_id\" binding:\"required\""
			Quantity  int       "json:\"quantity\" binding:\"required,min=1\""
		}{},
	}
	pid := uuid.New()
	req.Items = append(req.Items, struct {
		ProductID uuid.UUID `json:"product_id" binding:"required"`
		Quantity  int       `json:"quantity" binding:"required,min=1"`
	}{ProductID: pid, Quantity: 2})

	// Act
	err := svc.CreateOrder(context.Background(), "1", req)
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	// Assert SNS published
	if sns.publishedArn != "arn:aws:sns:eu-west-2:000000000000:order-events" {
		t.Fatalf("expected sns arn published, got %s", sns.publishedArn)
	}
	if len(sns.publishedMsg) == 0 {
		t.Fatalf("expected sns message to be published")
	}

	// Verify message is valid JSON
	var out map[string]interface{}
	if err := json.Unmarshal(sns.publishedMsg, &out); err != nil {
		t.Fatalf("sns published invalid json: %v", err)
	}
	if _, ok := out["items"]; !ok {
		t.Fatalf("sns payload missing items")
	}

	// small timing sanity
	time.Sleep(10 * time.Millisecond)
}
