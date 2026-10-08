package messaging

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestTopicRouting(t *testing.T) {
	assert.Equal(t, []string{"order-processing-queue", "promotion-order-queue"}, resolveQueues("arn:aws:sns:eu-west-2:000000000000:order-events"))
	assert.Equal(t, []string{"payment-events-queue"}, resolveQueues("payment-events"))
	assert.Equal(t, []string{"notification-queue"}, resolveQueues("auth-events"))
	assert.Equal(t, []string{"notification-queue"}, resolveQueues("notification-topic"))
	assert.Equal(t, []string{"custom-queue"}, resolveQueues("custom-queue"))
	assert.Equal(t, []string{"notification-queue"}, resolveQueues("http://localstack:4566/queue/us-east-1/000000000000/notification-queue"))
}

func TestMessagingWithDB(t *testing.T) {
	dsn := "postgres://postgres:admin@localhost:5432/ecommerce?sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skip("Postgres not available, skipping DB test")
		return
	}

	publisher := NewPGQueuePublisher(db, zap.NewNop())
	consumer := NewPGQueueConsumer(db, "test-unit-queue", zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	payload := `{"test_id": "12345", "action": "test"}`
	err = publisher.Publish(ctx, "test-unit-queue", []byte(payload))
	assert.NoError(t, err)

	received := make(chan string, 1)
	handler := func(ctx context.Context, body string) error {
		received <- body
		return nil
	}

	processed, err := consumer.pollOnce(ctx, handler)
	assert.NoError(t, err)
	assert.True(t, processed)

	select {
	case msg := <-received:
		assert.JSONEq(t, payload, msg)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message")
	}
}
