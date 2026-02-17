package aws

import "context"

// aws_stub.go
// Minimal placeholder to satisfy imports during CI when local replace points to ../../pkg/aws.
// These are no-op implementations — replace with real AWS integration when ready.

type AWSConfig struct{}

// LoadAWSConfig returns a minimal AWSConfig placeholder.
func LoadAWSConfig(ctx context.Context) (*AWSConfig, error) {
	return &AWSConfig{}, nil
}

type SecretsClient struct{}

// NewSecretsClient returns a minimal SecretsClient placeholder.
func NewSecretsClient(cfg *AWSConfig) *SecretsClient {
	return &SecretsClient{}
}

// GetSecret is a no-op that returns an empty value and no error.
func (s *SecretsClient) GetSecret(ctx context.Context, name string) (string, error) {
	return "", nil
}

// SNSPublisher is a minimal interface used by services to publish SNS messages.
type SNSPublisher interface {
	Publish(ctx context.Context, topicArn string, payload []byte) error
}

// SNSClient is a no-op SNS publisher used during CI/local development.
type SNSClient struct{}

// NewSNSClient returns a minimal SNSClient placeholder.
func NewSNSClient(cfg *AWSConfig) *SNSClient {
	return &SNSClient{}
}

// Publish is a no-op that simulates publishing to SNS and returns no error.
func (s *SNSClient) Publish(ctx context.Context, topicArn string, payload []byte) error {
	return nil
}

// SQSConsumer is a minimal no-op SQS helper used by services to poll/send messages.
type SQSConsumer struct {
	QueueURL string
}

// NewSQSConsumer returns a minimal SQSConsumer placeholder.
func NewSQSConsumer(cfg *AWSConfig, queueURL string) *SQSConsumer {
	return &SQSConsumer{QueueURL: queueURL}
}

// StartPolling is a no-op implementation that should start polling in a real implementation.
// It accepts a callback which would be invoked for each received message.
func (s *SQSConsumer) StartPolling(ctx context.Context, handler func(ctx context.Context, body string) error) error {
	return nil
}

// SendMessage is a no-op that simulates sending a message to SQS.
func (s *SQSConsumer) SendMessage(ctx context.Context, body string) error {
	return nil
}

// GetQueueURL is a helper that would normally resolve a queue name to its URL.
// In this stub it returns an empty string and no error.
func GetQueueURL(ctx context.Context, cfg *AWSConfig, queueName string) (string, error) {
	return "", nil
}

// GeneratePresignedPutURL returns a presigned S3 PUT URL and the HTTP method to use.
// This is a stub and returns an empty URL and "PUT" as method.
func GeneratePresignedPutURL(ctx context.Context, cfg *AWSConfig, bucket, key string, expires int64) (string, string, error) {
	return "", "PUT", nil
}
