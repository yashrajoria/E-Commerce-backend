package aws

import (
	"context"
	"fmt"
	"log"
	"strings"

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
		// If topic doesn't exist, attempt to create it (useful for LocalStack/dev)
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "Topic does not exist") {
			parts := strings.Split(topicArn, ":")
			if len(parts) > 0 {
				topicName := parts[len(parts)-1]
				ctIn := &sns.CreateTopicInput{Name: awsString(topicName)}
				ctOut, cerr := s.client.CreateTopic(ctx, ctIn)
				if cerr != nil {
					return fmt.Errorf("sns create topic failed for %s: %w", topicName, cerr)
				}
				// retry publish using the created topic ARN
				input.TopicArn = ctOut.TopicArn
				if _, perr := s.client.Publish(ctx, input); perr != nil {
					return fmt.Errorf("sns publish failed after create for topic %s: %w", topicArn, perr)
				}
				log.Printf("[SNS][PUBLISH] created topic %s and published", *ctOut.TopicArn)
				return nil
			}
		}
		return fmt.Errorf("sns publish failed for topic %s: %w", topicArn, err)
	}
	return nil
}

func awsString(s string) *string { return &s }
