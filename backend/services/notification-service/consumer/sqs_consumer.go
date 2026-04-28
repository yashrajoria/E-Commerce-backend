package consumer

import (
	"context"
	"encoding/json"
	"notification-service/models"
	"notification-service/services"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	awspkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"go.uber.org/zap"
)

type SQSConsumer struct {
	client   *sqs.Client
	queueURL string
	service  services.NotificationService
	logger   *zap.Logger
}

func NewSQSConsumer(svc services.NotificationService, logger *zap.Logger) (*SQSConsumer, error) {

	queueURL := os.Getenv("SQS_QUEUE_URL")
	if queueURL == "" {
		// Backward/compose-compat fallback
		queueURL = os.Getenv("NOTIFICATION_SQS_QUEUE_URL")
	}

	cfg, err := awspkg.LoadConfig(context.Background())
	if err != nil {
		return nil, err
	}

	return &SQSConsumer{
		client:   sqs.NewFromConfig(cfg),
		queueURL: queueURL,
		service:  svc,
		logger:   logger,
	}, nil
}

func (c *SQSConsumer) Start(ctx context.Context) {
	c.logger.Info("SQS consumer started", zap.String("queue", c.queueURL))
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("SQS consumer shutting down")
			return
		default:
			c.poll(ctx)
		}
	}
}

func (c *SQSConsumer) poll(ctx context.Context) {
	output, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(c.queueURL),
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     5, // long polling
	})
	if err != nil {
		c.logger.Error("SQS receive error", zap.Error(err))
		time.Sleep(5 * time.Second)
		return
	}

	for _, msg := range output.Messages {
		c.processMessage(ctx, msg.Body, msg.ReceiptHandle)
	}
}

// snsEnvelope unwraps the SNS → SQS message wrapper
type snsEnvelope struct {
	Message string `json:"Message"`
}

func (c *SQSConsumer) processMessage(ctx context.Context, body *string, receiptHandle *string) {
	if body == nil || *body == "" {
		c.logger.Error("received empty SQS message body")
		// Don't delete; let it retry / get sent to DLQ if configured.
		return
	}
	if receiptHandle == nil || *receiptHandle == "" {
		c.logger.Error("received empty SQS receipt handle")
		return
	}

	// Step 1: unwrap SNS envelope
	var envelope snsEnvelope
	if err := json.Unmarshal([]byte(*body), &envelope); err != nil {
		c.logger.Error("failed to unmarshal SNS envelope", zap.Error(err))
		c.deleteMessage(ctx, receiptHandle) // unparseable — delete to avoid infinite loop
		return
	}

	// Step 2: unmarshal actual event payload
	var payload models.EventPayload
	if err := json.Unmarshal([]byte(envelope.Message), &payload); err != nil {
		c.logger.Error("failed to unmarshal event payload", zap.Error(err))
		c.deleteMessage(ctx, receiptHandle)
		return
	}

	// Step 3: process — do NOT delete on failure, let SQS retry
	if err := c.service.ProcessEvent(ctx, &payload); err != nil {
		c.logger.Error("failed to process event",
			zap.String("event_type", payload.EventType),
			zap.Error(err),
		)
		return // SQS will retry after visibility timeout
	}

	// Step 4: delete only on success
	c.deleteMessage(ctx, receiptHandle)
}

func (c *SQSConsumer) deleteMessage(ctx context.Context, receiptHandle *string) {
	_, err := c.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.queueURL),
		ReceiptHandle: receiptHandle,
	})
	if err != nil {
		c.logger.Error("failed to delete SQS message", zap.Error(err))
	}
}
