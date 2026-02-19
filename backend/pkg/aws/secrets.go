package aws

import (
	"context"
	"fmt"
	"sync"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type SecretsClient struct {
	client *secretsmanager.Client
	cache  map[string]string
	mu     sync.RWMutex
}

func NewSecretsClient(cfg sdkaws.Config) *SecretsClient {
	return &SecretsClient{
		client: secretsmanager.NewFromConfig(cfg),
		cache:  make(map[string]string),
	}
}

func (s *SecretsClient) GetSecret(ctx context.Context, name string) (string, error) {
	s.mu.RLock()
	if v, ok := s.cache[name]; ok {
		s.mu.RUnlock()
		return v, nil
	}
	s.mu.RUnlock()

	out, err := s.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: &name})
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s: %w", name, err)
	}
	if out.SecretString == nil {
		return "", fmt.Errorf("secret %s has no string value", name)
	}

	s.mu.Lock()
	s.cache[name] = *out.SecretString
	s.mu.Unlock()

	return *out.SecretString, nil
}
