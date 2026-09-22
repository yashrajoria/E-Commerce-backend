package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type fakeSQSOutboxClient struct {
	queueURL    string
	resolved    string
	messageBody string
	resolveErr  error
	sendErr     error
}

func (f *fakeSQSOutboxClient) GetQueueUrl(context.Context, *sqs.GetQueueUrlInput, ...func(*sqs.Options)) (*sqs.GetQueueUrlOutput, error) {
	if f.resolveErr != nil {
		return nil, f.resolveErr
	}
	return &sqs.GetQueueUrlOutput{QueueUrl: &f.queueURL}, nil
}

func (f *fakeSQSOutboxClient) SendMessage(_ context.Context, input *sqs.SendMessageInput, _ ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	f.resolved = *input.QueueUrl
	f.messageBody = *input.MessageBody
	return &sqs.SendMessageOutput{}, f.sendErr
}

func TestSQSOutboxPublisherResolvesQueueNames(t *testing.T) {
	client := &fakeSQSOutboxClient{queueURL: "https://sqs.local/queue"}
	publisher := &SQSOutboxPublisher{client: client}

	if err := publisher.Publish(context.Background(), "payment-request-queue", []byte("payload")); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if client.resolved != client.queueURL || client.messageBody != "payload" {
		t.Fatalf("unexpected send: url=%q body=%q", client.resolved, client.messageBody)
	}
}

func TestSQSOutboxPublisherUsesDirectQueueURL(t *testing.T) {
	client := &fakeSQSOutboxClient{}
	publisher := &SQSOutboxPublisher{client: client}
	queueURL := "http://localhost:4566/000000000000/orders"

	if err := publisher.Publish(context.Background(), queueURL, []byte("payload")); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if client.resolved != queueURL {
		t.Fatalf("expected direct queue URL, got %q", client.resolved)
	}
}

func TestSQSOutboxPublisherWrapsAWSFailures(t *testing.T) {
	client := &fakeSQSOutboxClient{resolveErr: errors.New("lookup failed")}
	publisher := &SQSOutboxPublisher{client: client}

	if err := publisher.Publish(context.Background(), "missing-queue", nil); err == nil {
		t.Fatal("expected queue resolution error")
	}

	client = &fakeSQSOutboxClient{sendErr: errors.New("send failed")}
	publisher = &SQSOutboxPublisher{client: client}
	if err := publisher.Publish(context.Background(), "http://localhost/queue", nil); err == nil {
		t.Fatal("expected send error")
	}
}
