package integration

import (
	"context"
	"os"
	"testing"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
)

// This test runs only when RUN_LOCALSTACK_INTEGRATION=true and an endpoint is available at AWS_ENDPOINT or default localhost:4566
func TestSNSPublish_LocalStack(t *testing.T) {
	if os.Getenv("RUN_LOCALSTACK_INTEGRATION") != "true" {
		t.Skip("skipping localstack integration test; set RUN_LOCALSTACK_INTEGRATION=true to run")
	}

	cfg, err := aws_pkg.LoadAWSConfig(context.Background())
	if err != nil {
		t.Fatalf("failed to load aws config: %v", err)
	}
	sns := aws_pkg.NewSNSClient(cfg)
	topic := os.Getenv("ORDER_SNS_TOPIC_ARN")
	if topic == "" {
		t.Fatalf("ORDER_SNS_TOPIC_ARN must be set for integration test")
	}
	if err := sns.Publish(context.Background(), topic, []byte(`{"test":"ok"}`)); err != nil {
		t.Fatalf("sns publish failed: %v", err)
	}
}
