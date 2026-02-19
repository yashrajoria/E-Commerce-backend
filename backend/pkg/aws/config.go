package aws

import (
	"context"
	"os"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

// LoadAWSConfig loads AWS config using environment defaults.
// This function intentionally avoids any local endpoint-specific resolver logic.
func LoadAWSConfig(ctx context.Context) (sdkaws.Config, error) {
	opts := []func(*config.LoadOptions) error{}
	if region := os.Getenv("AWS_REGION"); region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	return config.LoadDefaultConfig(ctx, opts...)
}
