package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/yashrajoria/common/messaging"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SNSPublisher represents sns publisher.
type SNSPublisher struct {
	publisher messaging.Publisher
	topic     string
}

// NewSNSPublisherWithDB creates and initializes a new SNSPublisherWithDB instance.
func NewSNSPublisherWithDB(db *gorm.DB) *SNSPublisher {
	topic := os.Getenv("NOTIFICATION_SNS_TOPIC_ARN")
	if topic == "" {
		topic = os.Getenv("AUTH_SNS_TOPIC_ARN")
	}
	if topic == "" {
		topic = "notification-queue"
	}

	return &SNSPublisher{
		publisher: messaging.NewPGQueuePublisher(db, zap.L()),
		topic:     topic,
	}
}

// NewSNSPublisher creates and initializes a new SNSPublisher instance.
func NewSNSPublisher(ctx context.Context) (*SNSPublisher, error) {
	// Fallback constructor
	topic := os.Getenv("NOTIFICATION_SNS_TOPIC_ARN")
	if topic == "" {
		topic = os.Getenv("AUTH_SNS_TOPIC_ARN")
	}
	if topic == "" {
		topic = "notification-queue"
	}
	return &SNSPublisher{
		topic: topic,
	}, nil
}

// Publish dispatches the message payload to the configured broker.
func (p *SNSPublisher) Publish(ctx context.Context, eventType string, payload map[string]interface{}) error {
	if p.publisher == nil {
		zap.L().Warn("Publisher not configured, skipping event publish", zap.String("eventType", eventType))
		return nil
	}

	message := map[string]interface{}{
		"event_type": eventType,
		"data":       payload,
	}

	msgBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	return p.publisher.Publish(ctx, p.topic, msgBytes)
}
