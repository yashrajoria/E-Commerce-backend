package dynamodb

import (
	"context"
	"fmt"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// NewClient loads AWS config and returns a DynamoDB client.
func NewClient(ctx context.Context) (*dynamodb.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}
	return dynamodb.NewFromConfig(cfg), nil
}

// NewClientFromConfig accepts an AWS SDK config and returns a DynamoDB client.
func NewClientFromConfig(cfg sdkaws.Config) *dynamodb.Client {
	return dynamodb.NewFromConfig(cfg)
}
