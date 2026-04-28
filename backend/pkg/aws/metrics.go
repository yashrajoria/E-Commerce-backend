package aws

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// MetricsClient wraps AWS CloudWatch Metrics operations
type MetricsClient struct {
	client    *cloudwatch.Client
	namespace string
	enabled   bool
}

// NewMetricsClient creates a new CloudWatch Metrics client
func NewMetricsClient(ctx context.Context) (*MetricsClient, error) {
	enabled := os.Getenv("CLOUDWATCH_ENABLED") == "true"

	cfg, err := LoadAWSConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}

	client := cloudwatch.NewFromConfig(cfg)

	namespace := os.Getenv("CLOUDWATCH_NAMESPACE")
	if namespace == "" {
		namespace = "ECommerce"
	}

	return &MetricsClient{
		client:    client,
		namespace: namespace,
		enabled:   enabled,
	}, nil
}

// PutMetric sends a single metric data point to CloudWatch
func (m *MetricsClient) PutMetric(ctx context.Context, metricName string, value float64, unit types.StandardUnit, dimensions map[string]string) error {
	if !m.enabled {
		return nil
	}

	dims := make([]types.Dimension, 0, len(dimensions))
	for k, v := range dimensions {
		dims = append(dims, types.Dimension{
			Name:  aws.String(k),
			Value: aws.String(v),
		})
	}

	_, err := m.client.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{
		Namespace: aws.String(m.namespace),
		MetricData: []types.MetricDatum{
			{
				MetricName: aws.String(metricName),
				Value:      aws.Float64(value),
				Unit:       unit,
				Timestamp:  aws.Time(time.Now()),
				Dimensions: dims,
			},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to put metric: %w", err)
	}

	return nil
}

// PutMetricBatch sends multiple metric data points to CloudWatch
func (m *MetricsClient) PutMetricBatch(ctx context.Context, metrics []types.MetricDatum) error {
	if !m.enabled || len(metrics) == 0 {
		return nil
	}

	// CloudWatch allows max 1000 metrics per batch, but we'll use 20 for safety
	batchSize := 20
	for i := 0; i < len(metrics); i += batchSize {
		end := i + batchSize
		if end > len(metrics) {
			end = len(metrics)
		}

		_, err := m.client.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{
			Namespace:  aws.String(m.namespace),
			MetricData: metrics[i:end],
		})

		if err != nil {
			return fmt.Errorf("failed to put metric batch: %w", err)
		}
	}

	return nil
}

// RecordCount increments a counter metric
func (m *MetricsClient) RecordCount(ctx context.Context, metricName string, dimensions map[string]string) error {
	return m.PutMetric(ctx, metricName, 1, types.StandardUnitCount, dimensions)
}

// RecordLatency records a latency/duration metric in milliseconds
func (m *MetricsClient) RecordLatency(ctx context.Context, metricName string, duration time.Duration, dimensions map[string]string) error {
	return m.PutMetric(ctx, metricName, float64(duration.Milliseconds()), types.StandardUnitMilliseconds, dimensions)
}

// RecordValue records a generic value metric
func (m *MetricsClient) RecordValue(ctx context.Context, metricName string, value float64, dimensions map[string]string) error {
	return m.PutMetric(ctx, metricName, value, types.StandardUnitNone, dimensions)
}

// IsEnabled returns whether CloudWatch metrics are enabled
func (m *MetricsClient) IsEnabled() bool {
	return m.enabled
}

// Common metric names for standardization
const (
	// HTTP metrics
	MetricHTTPRequests = "HTTPRequests"
	MetricHTTPErrors   = "HTTPErrors"
	MetricHTTPLatency  = "HTTPLatency"
	MetricHTTP4xx      = "HTTP4xxErrors"
	MetricHTTP5xx      = "HTTP5xxErrors"

	// Business metrics
	MetricOrdersCreated      = "OrdersCreated"
	MetricOrdersCompleted    = "OrdersCompleted"
	MetricOrdersFailed       = "OrdersFailed"
	MetricPaymentSucceeded   = "PaymentSucceeded"
	MetricPaymentFailed      = "PaymentFailed"
	MetricProductsCreated    = "ProductsCreated"
	MetricInventoryReserved  = "InventoryReserved"
	MetricInventoryReleased  = "InventoryReleased"
	MetricInventoryConfirmed = "InventoryConfirmed"
	MetricInventoryLow       = "InventoryLowStock"
	MetricCartCheckouts      = "CartCheckouts"

	// System metrics
	MetricDatabaseLatency = "DatabaseLatency"
	MetricCacheHits       = "CacheHits"
	MetricCacheMisses     = "CacheMisses"
	MetricSQSMessages     = "SQSMessagesProcessed"
)
