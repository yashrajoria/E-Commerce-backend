package consumer

import (
	"context"
	"encoding/json"
	"notification-service/models"
	"notification-service/services"
	"os"

	"github.com/google/uuid"
	"github.com/yashrajoria/common/messaging"
	"github.com/yashrajoria/common/telemetry"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type SQSConsumer struct {
	consumer messaging.Consumer
	service  services.NotificationService
	logger   *zap.Logger
}

func NewSQSConsumerWithDB(db *gorm.DB, svc services.NotificationService, logger *zap.Logger) (*SQSConsumer, error) {
	queueName := os.Getenv("SQS_QUEUE_URL")
	if queueName == "" {
		queueName = os.Getenv("NOTIFICATION_SQS_QUEUE_URL")
	}
	if queueName == "" {
		queueName = "notification-queue"
	}

	return &SQSConsumer{
		consumer: messaging.NewPGQueueConsumer(db, queueName, logger),
		service:  svc,
		logger:   logger,
	}, nil
}

func (c *SQSConsumer) Start(ctx context.Context) {
	if os.Getenv("ENABLE_SQS_CONSUMER") == "false" {
		c.logger.Info("Notification queue consumer disabled (ENABLE_SQS_CONSUMER=false)")
		return
	}
	c.logger.Info("Notification queue consumer started")
	go func() {
		_ = c.consumer.StartPolling(ctx, c.handleMessage)
	}()
}

func (c *SQSConsumer) handleMessage(ctx context.Context, body string) error {
	ctx, endSpan := telemetry.StartSQSConsumerSpan(ctx, "notification-queue", "")
	defer endSpan()

	var payload models.EventPayload
	var envelope struct {
		Message string `json:"Message"`
	}

	if err := json.Unmarshal([]byte(body), &envelope); err == nil && envelope.Message != "" {
		_ = json.Unmarshal([]byte(envelope.Message), &payload)
	} else {
		_ = json.Unmarshal([]byte(body), &payload)
	}

	if payload.EventType == "" {
		var generic map[string]interface{}
		if err := json.Unmarshal([]byte(body), &generic); err == nil {
			if et, ok := generic["event_type"].(string); ok {
				payload.EventType = et
			}
			if d, ok := generic["data"].(map[string]interface{}); ok {
				payload.Data = d
			}
		}
	}

	payload.CorrelationID = ensureCorrelationID(payload.CorrelationID, payload.EventID)
	c.logger.Info("notification event received", zap.String("event_type", payload.EventType), zap.String("correlation_id", payload.CorrelationID))

	return c.service.ProcessEvent(ctx, &payload)
}

func ensureCorrelationID(correlationID, eventID string) string {
	if correlationID != "" {
		return correlationID
	}
	if eventID != "" {
		return uuid.NewSHA1(uuid.NameSpaceOID, []byte("notification:"+eventID)).String()
	}
	return uuid.NewString()
}
