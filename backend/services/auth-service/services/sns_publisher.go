package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	awspkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
)

type SNSPublisher struct {
	client   *sns.Client
	topicARN string
}

func NewSNSPublisher(ctx context.Context) (*SNSPublisher, error) {
	cfg, err := awspkg.LoadConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Prefer the dedicated notification topic so notification-service receives
	// auth events through the unified notification pipeline.
	topicARN := os.Getenv("NOTIFICATION_SNS_TOPIC_ARN")
	if topicARN == "" {
		topicARN = os.Getenv("AUTH_SNS_TOPIC_ARN")
	}
	if topicARN == "" {
		return nil, fmt.Errorf("NOTIFICATION_SNS_TOPIC_ARN/AUTH_SNS_TOPIC_ARN not set")
	}

	return &SNSPublisher{
		client:   sns.NewFromConfig(cfg),
		topicARN: topicARN,
	}, nil
}

func (p *SNSPublisher) Publish(ctx context.Context, eventType string, payload map[string]interface{}) error {
	message := map[string]interface{}{
		"event_type": eventType,
		"data":       payload,
	}

	msgBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	_, err = p.client.Publish(ctx, &sns.PublishInput{
		TopicArn: sdkaws.String(p.topicARN),
		Message:  sdkaws.String(string(msgBytes)),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"event_type": {
				DataType:    sdkaws.String("String"),
				StringValue: sdkaws.String(eventType),
			},
		},
	})
	return err
}
