package aws

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// LoadAWSConfig loads AWS SDK v2 configuration, honoring AWS_REGION and
// optional AWS_ENDPOINT_URL (for LocalStack). Returns an aws.Config.
func LoadAWSConfig(ctx context.Context) (aws.Config, error) {
	region := os.Getenv("AWS_REGION")
	opts := []func(*awsconfig.LoadOptions) error{}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}

	// If an explicit endpoint URL is provided (e.g. LocalStack), configure
	// a custom endpoint resolver that points every service to that URL.
	if ep := os.Getenv("AWS_ENDPOINT_URL"); ep != "" {
		resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, _ ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{PartitionID: "aws", URL: ep, SigningRegion: region}, nil
		})
		opts = append(opts, awsconfig.WithEndpointResolverWithOptions(resolver))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, err
	}
	return cfg, nil
}

// SecretsClient wraps AWS Secrets Manager client.
type SecretsClient struct{ client *secretsmanager.Client }

func NewSecretsClient(cfg aws.Config) *SecretsClient {
	return &SecretsClient{client: secretsmanager.NewFromConfig(cfg)}
}

// GetSecret retrieves a secret string from AWS Secrets Manager.
func (s *SecretsClient) GetSecret(ctx context.Context, name string) (string, error) {
	if s == nil || s.client == nil {
		return "", errors.New("secrets client not configured")
	}
	out, err := s.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(name)})
	if err != nil {
		return "", err
	}
	if out.SecretString != nil {
		return *out.SecretString, nil
	}
	return "", nil
}

// SNSPublisher is the interface services use for publishing SNS messages.
type SNSPublisher interface {
	Publish(ctx context.Context, topicArn string, payload []byte) error
}

// SNSClient wraps AWS SNS client.
type SNSClient struct{ client *sns.Client }

func NewSNSClient(cfg aws.Config) *SNSClient {
	return &SNSClient{client: sns.NewFromConfig(cfg)}
}

func (s *SNSClient) Publish(ctx context.Context, topicArn string, payload []byte) error {
	if s == nil || s.client == nil {
		return errors.New("sns client not configured")
	}
	msg := string(payload)
	_, err := s.client.Publish(ctx, &sns.PublishInput{TopicArn: aws.String(topicArn), Message: aws.String(msg)})
	return err
}

// SQSConsumer is a simple wrapper around AWS SQS used for send/receive.
type SQSConsumer struct{ client *sqs.Client; QueueURL string }

func NewSQSConsumer(cfg aws.Config, queueURL string) *SQSConsumer {
	return &SQSConsumer{client: sqs.NewFromConfig(cfg), QueueURL: queueURL}
}

// StartPolling starts a simple polling loop that calls handler for each message.
// This implementation is intentionally minimal; it should be enhanced for
// production (visibility timeout, backoff, batching, long-poll, delete on success).
func (s *SQSConsumer) StartPolling(ctx context.Context, handler func(ctx context.Context, body string) error) error {
	if s == nil || s.client == nil || s.QueueURL == "" {
		return errors.New("sqs consumer not configured")
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			out, err := s.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{QueueUrl: aws.String(s.QueueURL), MaxNumberOfMessages: 5, WaitTimeSeconds: 10})
			if err != nil || out == nil || len(out.Messages) == 0 {
				time.Sleep(1 * time.Second)
				continue
			}
			for _, m := range out.Messages {
				_ = handler(ctx, aws.ToString(m.Body))
				// Best-effort delete (ignore errors)
				if m.ReceiptHandle != nil {
					_, _ = s.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: aws.String(s.QueueURL), ReceiptHandle: m.ReceiptHandle})
				}
			}
		}
	}()
	return nil
}

func (s *SQSConsumer) SendMessage(ctx context.Context, body string) error {
	if s == nil || s.client == nil || s.QueueURL == "" {
		return errors.New("sqs consumer not configured")
	}
	_, err := s.client.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: aws.String(s.QueueURL), MessageBody: aws.String(body)})
	return err
}

// GetQueueURL resolves a queue name to its URL.
func GetQueueURL(ctx context.Context, cfg aws.Config, queueName string) (string, error) {
	client := sqs.NewFromConfig(cfg)
	out, err := client.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{QueueName: aws.String(queueName)})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.QueueUrl), nil
}

// GeneratePresignedPutURL creates a presigned S3 PUT URL with expiry seconds.
func GeneratePresignedPutURL(ctx context.Context, cfg aws.Config, bucket, key string, expires int64) (string, string, error) {
	client := s3.NewFromConfig(cfg)
	presigner := s3.NewPresignClient(client)
	dur := time.Duration(expires) * time.Second
	req, err := presigner.PresignPutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}, s3.WithPresignExpires(dur))
	if err != nil {
		return "", "", err
	}
	return req.URL, "PUT", nil
}

// NewDynamoClient returns a DynamoDB client from config.
func NewDynamoClient(cfg aws.Config) *dynamodb.Client { return dynamodb.NewFromConfig(cfg) }

// NewCloudWatchClient returns a CloudWatch client from config.
func NewCloudWatchClient(cfg aws.Config) *cloudwatch.Client { return cloudwatch.NewFromConfig(cfg) }

// NewS3Uploader returns a simple S3 uploader using manager.Uploader.
func NewS3Uploader(cfg aws.Config) *manager.Uploader { return manager.NewUploader(s3.NewFromConfig(cfg)) }
