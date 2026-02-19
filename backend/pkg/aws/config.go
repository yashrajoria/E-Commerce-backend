package aws

import (
	"context"
	"fmt"
	"os"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

// LoadAWSConfig loads AWS config and supports LocalStack endpoint via AWS_S3_ENDPOINT, AWS_SQS_ENDPOINT or AWS_ENDPOINT env vars.
// If those are set the function adds an endpoint resolver so SDK clients target the LocalStack URL instead of AWS.
func LoadAWSConfig(ctx context.Context) (sdkaws.Config, error) {
	// Allow env-based credentials/region to be used by LoadDefaultConfig
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return cfg, fmt.Errorf("failed to load aws config: %w", err)
	}

	// Debug: print loaded region from SDK config for local troubleshooting
	loadedRegion := cfg.Region
	if loadedRegion == "" {
		loadedRegion = os.Getenv("AWS_REGION")
	}
	fmt.Printf("[AWS DEBUG] loaded cfg.Region=%q (fallback AWS_REGION=%q)\n", cfg.Region, os.Getenv("AWS_REGION"))

	// Prefer service-specific env var then generic AWS_ENDPOINT
	endpoint := os.Getenv("AWS_SQS_ENDPOINT")
	if endpoint == "" {
		endpoint = os.Getenv("AWS_S3_ENDPOINT")
	}
	if endpoint == "" {
		endpoint = os.Getenv("AWS_ENDPOINT")
	}

	if endpoint != "" {
		// Determine signing region: prefer loaded cfg region, fallback to env AWS_REGION or provided region
		signingRegion := cfg.Region
		if signingRegion == "" {
			signingRegion = os.Getenv("AWS_REGION")
		}

		// Create a resolver that returns the same endpoint for all services so LocalStack edge port is used.
		resolver := sdkaws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (sdkaws.Endpoint, error) {
			sr := signingRegion
			if sr == "" {
				sr = region
			}
			return sdkaws.Endpoint{
				URL:               endpoint,
				SigningRegion:     sr,
				HostnameImmutable: true,
			}, nil
		})
		// Merge with existing cfg by overriding EndpointResolverWithOptions
		cfg.EndpointResolverWithOptions = resolver

		// Debug: print resolved endpoint and signing region (helpful when using LocalStack)
		fmt.Printf("[AWS DEBUG] custom endpoint configured: %s, signingRegion=%q\n", endpoint, signingRegion)
	}

	return cfg, nil
}
