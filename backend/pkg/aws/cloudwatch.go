package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// CloudWatchLogsClient wraps AWS CloudWatch Logs operations
type CloudWatchLogsClient struct {
	client        *cloudwatchlogs.Client
	logGroupName  string
	logStreamName string
	sequenceToken *string
	enabled       bool
}

// NewCloudWatchLogsClient creates a new CloudWatch Logs client
func NewCloudWatchLogsClient(ctx context.Context, serviceName string) (*CloudWatchLogsClient, error) {
	// Check if CloudWatch is enabled (disabled by default for local dev)
	enabled := os.Getenv("CLOUDWATCH_ENABLED") == "true"

	cfg, err := LoadAWSConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}

	client := cloudwatchlogs.NewFromConfig(cfg)

	logGroupName := os.Getenv("CLOUDWATCH_LOG_GROUP")
	if logGroupName == "" {
		logGroupName = "/ecommerce/services"
	}

	logStreamName := fmt.Sprintf("%s-%d", serviceName, time.Now().Unix())

	cwClient := &CloudWatchLogsClient{
		client:        client,
		logGroupName:  logGroupName,
		logStreamName: logStreamName,
		enabled:       enabled,
	}

	if enabled {
		// Ensure log group exists
		if err := cwClient.ensureLogGroup(ctx); err != nil {
			return nil, fmt.Errorf("failed to ensure log group: %w", err)
		}

		// Create log stream
		if err := cwClient.createLogStream(ctx); err != nil {
			return nil, fmt.Errorf("failed to create log stream: %w", err)
		}
	}

	return cwClient, nil
}

// ensureLogGroup creates the log group if it doesn't exist
func (c *CloudWatchLogsClient) ensureLogGroup(ctx context.Context) error {
	_, err := c.client.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(c.logGroupName),
	})
	if err != nil {
		// ResourceAlreadyExistsException is expected and OK
		var existsErr *types.ResourceAlreadyExistsException
		if !errors.As(err, &existsErr) {
			return err
		}
	}

	// Set retention policy (30 days)
	_, err = c.client.PutRetentionPolicy(ctx, &cloudwatchlogs.PutRetentionPolicyInput{
		LogGroupName:    aws.String(c.logGroupName),
		RetentionInDays: aws.Int32(30),
	})
	if err != nil {
		return fmt.Errorf("failed to set retention policy: %w", err)
	}

	return nil
}

// createLogStream creates a new log stream
func (c *CloudWatchLogsClient) createLogStream(ctx context.Context) error {
	_, err := c.client.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  aws.String(c.logGroupName),
		LogStreamName: aws.String(c.logStreamName),
	})
	return err
}

// PutLogEvents sends log events to CloudWatch Logs
func (c *CloudWatchLogsClient) PutLogEvents(ctx context.Context, events []types.InputLogEvent) error {
	if !c.enabled || len(events) == 0 {
		return nil
	}

	input := &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(c.logGroupName),
		LogStreamName: aws.String(c.logStreamName),
		LogEvents:     events,
		SequenceToken: c.sequenceToken,
	}

	output, err := c.client.PutLogEvents(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to put log events: %w", err)
	}

	// Update sequence token for next call
	c.sequenceToken = output.NextSequenceToken
	return nil
}

// Write implements io.Writer interface for log shipping
func (c *CloudWatchLogsClient) Write(p []byte) (n int, err error) {
	if !c.enabled {
		return len(p), nil
	}

	ctx := context.Background()
	event := types.InputLogEvent{
		Message:   aws.String(string(p)),
		Timestamp: aws.Int64(time.Now().UnixMilli()),
	}

	if err := c.PutLogEvents(ctx, []types.InputLogEvent{event}); err != nil {
		// Log error but don't fail the write
		fmt.Fprintf(os.Stderr, "CloudWatch write error: %v\n", err)
		return len(p), nil
	}

	return len(p), nil
}

// IsEnabled returns whether CloudWatch logging is enabled
func (c *CloudWatchLogsClient) IsEnabled() bool {
	return c.enabled
}
