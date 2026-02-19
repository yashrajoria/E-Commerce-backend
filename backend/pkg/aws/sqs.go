package aws

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// SQSConsumer provides methods for consuming messages from SQS queues
type SQSConsumer struct {
	client   *sqs.Client
	queueURL string
}

// NewSQSConsumer creates a new SQS consumer for the given queue URL
func NewSQSConsumer(cfg aws.Config, queueURL string) *SQSConsumer {
	return &SQSConsumer{
		client:   sqs.NewFromConfig(cfg),
		queueURL: queueURL,
	}
}

// MessageHandler is a function that processes an SQS message
type MessageHandler func(ctx context.Context, body string) error

// StartPolling polls SQS for messages and processes them with the handler
// Runs indefinitely until context is cancelled
func (c *SQSConsumer) StartPolling(ctx context.Context, handler MessageHandler) error {
	log.Printf("Starting SQS polling on queue: %s", c.queueURL)

	for {
		select {
		case <-ctx.Done():
			log.Println("SQS polling stopped")
			return ctx.Err()
		default:
			if err := c.pollOnce(ctx, handler); err != nil {
				log.Printf("Error polling SQS: %v", err)
			}
		}
	}
}

func (c *SQSConsumer) pollOnce(ctx context.Context, handler MessageHandler) error {
	result, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            &c.queueURL,
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     20, // Long polling
		VisibilityTimeout:   30,
	})
	if err != nil {
		return fmt.Errorf("failed to receive messages: %w", err)
	}

	for _, msg := range result.Messages {
		if msg.Body == nil {
			continue
		}

		// Process message
		if err := handler(ctx, *msg.Body); err != nil {
			log.Printf("Failed to process message: %v", err)
			// Message will become visible again after VisibilityTimeout
			continue
		}

		// Delete message after successful processing
		if _, err := c.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
			QueueUrl:      &c.queueURL,
			ReceiptHandle: msg.ReceiptHandle,
		}); err != nil {
			log.Printf("Failed to delete message: %v", err)
		}
	}

	return nil
}

// GetQueueURL retrieves the URL for a queue name
func GetQueueURL(ctx context.Context, cfg aws.Config, queueName string) (string, error) {
	client := sqs.NewFromConfig(cfg)
	result, err := client.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: &queueName,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get queue URL: %w", err)
	}
	return *result.QueueUrl, nil
}

// SendMessage sends a single message to the queue
func (c *SQSConsumer) SendMessage(ctx context.Context, body string) error {
	_, err := c.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    &c.queueURL,
		MessageBody: &body,
	})
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	return nil
}

// SendMessageBatch sends multiple messages to the queue
func (c *SQSConsumer) SendMessageBatch(ctx context.Context, messages []string) error {
	if len(messages) == 0 {
		return nil
	}

	// SQS batch limit is 10
	for i := 0; i < len(messages); i += 10 {
		end := i + 10
		if end > len(messages) {
			end = len(messages)
		}

		var entries []types.SendMessageBatchRequestEntry
		for j, msg := range messages[i:end] {
			id := fmt.Sprintf("msg-%d", j)
			body := msg
			entries = append(entries, types.SendMessageBatchRequestEntry{
				Id:          &id,
				MessageBody: &body,
			})
		}

		_, err := c.client.SendMessageBatch(ctx, &sqs.SendMessageBatchInput{
			QueueUrl: &c.queueURL,
			Entries:  entries,
		})
		if err != nil {
			return fmt.Errorf("failed to send batch: %w", err)
		}
	}

	return nil
}
