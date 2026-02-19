package aws

import (
	"context"
	"fmt"
	"log"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

// SNSPublisher is a minimal interface for publishing messages to SNS.
type SNSPublisher interface {
	Publish(ctx context.Context, topicArn string, message []byte) error
}

type SNSClient struct {
	client *sns.Client
}

func NewSNSClient(cfg sdkaws.Config) *SNSClient {
	return &SNSClient{client: sns.NewFromConfig(cfg)}
}

// Publish publishes a raw message to the given SNS topic ARN.
func (s *SNSClient) Publish(ctx context.Context, topicArn string, message []byte) error {
	// Log the TopicArn and message size for debugging (safe in local/dev)
	log.Printf("[SNS][PUBLISH] topicArn=%q message_len=%d", topicArn, len(message))

	if topicArn == "" {
		return fmt.Errorf("empty topicArn")
	}
	input := &sns.PublishInput{
		TopicArn: &topicArn,
		Message:  awsString(string(message)),
	}
	_, err := s.client.Publish(ctx, input)
	if err != nil {
		return fmt.Errorf("sns publish failed for topic %s: %w", topicArn, err)
	}
	return nil
}

func awsString(s string) *string { return &s }
