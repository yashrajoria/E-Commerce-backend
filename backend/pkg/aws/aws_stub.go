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
