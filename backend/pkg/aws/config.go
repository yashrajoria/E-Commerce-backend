package aws

import (
	"context"
	"os"
	"strings"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// IsLocalStack reports whether the service should connect to LocalStack instead of real AWS.
// Controlled by the USE_LOCALSTACK environment variable (set to "true" to enable).
func IsLocalStack() bool {
	return strings.EqualFold(os.Getenv("USE_LOCALSTACK"), "true")
}

// LocalStackEndpoint returns the LocalStack base endpoint URL.
// Reads LOCALSTACK_ENDPOINT from the environment; defaults to http://localhost:4566.
func LocalStackEndpoint() string {
	if ep := os.Getenv("LOCALSTACK_ENDPOINT"); ep != "" {
		return ep
	}
	return "http://localhost:4566"
}

// LoadConfig is the primary entry point for obtaining an AWS SDK config.
// It automatically routes to LocalStack when USE_LOCALSTACK=true, otherwise
// it uses real AWS credentials and region from the environment.
func LoadConfig(ctx context.Context) (sdkaws.Config, error) {
	if IsLocalStack() {
		return loadLocalStackConfig(ctx)
	}
	return loadAWSConfig(ctx)
}

// LoadAWSConfig is kept for backward compatibility and now delegates to LoadConfig.
// Existing callers do not need to change.
func LoadAWSConfig(ctx context.Context) (sdkaws.Config, error) {
	return LoadConfig(ctx)
}

// loadAWSConfig loads a real AWS config using environment credentials and region.
func loadAWSConfig(ctx context.Context) (sdkaws.Config, error) {
	opts := []func(*config.LoadOptions) error{}
	if region := os.Getenv("AWS_REGION"); region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	return config.LoadDefaultConfig(ctx, opts...)
}

// loadLocalStackConfig loads an AWS SDK config that points all service calls at LocalStack.
// It uses:
//   - LOCALSTACK_ENDPOINT (or http://localhost:4566) as the base endpoint.
//   - AWS_REGION (or "us-east-1") as the signing region.
//   - Static dummy credentials ("test"/"test") accepted by LocalStack.
func loadLocalStackConfig(ctx context.Context) (sdkaws.Config, error) {
	endpoint := LocalStackEndpoint()

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	return config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		// Route every AWS service call to the LocalStack endpoint.
		config.WithBaseEndpoint(endpoint),
		// LocalStack does not validate credentials; use static dummies so the SDK
		// never attempts an EC2 metadata / IAM role credential lookup.
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
}
