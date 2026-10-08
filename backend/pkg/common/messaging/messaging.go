package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Publisher publishes messages to a named queue or fan-out topic.
type Publisher interface {
	Publish(ctx context.Context, destination string, message []byte) error
}

// Consumer consumes messages from a named queue.
type Consumer interface {
	Start(ctx context.Context, handler func(ctx context.Context, body string) error)
	StartPolling(ctx context.Context, handler func(ctx context.Context, body string) error) error
}

// SNSPublisher alias for backward compatibility.
type SNSPublisher = Publisher

// Topic routing table: routes published topics to subscribed queues.
var topicRoutes = map[string][]string{
	"order-events":        {"order-processing-queue", "promotion-order-queue"},
	"payment-events":      {"payment-events-queue"},
	"auth-events":         {"notification-queue"},
	"notification-events": {"notification-queue"},
	"notification-topic":  {"notification-queue"},
}

// PGQueuePublisher implements Publisher backed by PostgreSQL.
type PGQueuePublisher struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewPGQueuePublisher creates a new PostgreSQL-backed message publisher.
func NewPGQueuePublisher(db *gorm.DB, logger *zap.Logger) *PGQueuePublisher {
	if logger == nil {
		logger = zap.L()
	}
	return &PGQueuePublisher{db: db, logger: logger}
}

// Publish routes and writes the message to the target queue(s) in Postgres.
func (p *PGQueuePublisher) Publish(ctx context.Context, destination string, message []byte) error {
	queues := resolveQueues(destination)
	if len(queues) == 0 {
		return fmt.Errorf("could not resolve destination %q to any queue", destination)
	}

	for _, queueName := range queues {
		err := p.db.WithContext(ctx).Exec(`
			INSERT INTO message_queues (queue_name, payload, status, visible_at, attempts, max_attempts, created_at, updated_at)
			VALUES (?, ?::jsonb, 'pending', NOW(), 0, 5, NOW(), NOW())
		`, queueName, string(message)).Error

		if err != nil {
			p.logger.Error("Failed to enqueue message", zap.String("queue", queueName), zap.Error(err))
			return fmt.Errorf("failed to enqueue to %q: %w", queueName, err)
		}
		p.logger.Debug("Enqueued message to Postgres queue", zap.String("queue", queueName), zap.Int("len", len(message)))
	}

	return nil
}

// PGQueueConsumer implements Consumer backed by PostgreSQL SKIP LOCKED polling.
type PGQueueConsumer struct {
	db        *gorm.DB
	queueName string
	logger    *zap.Logger
	pollDelay time.Duration
}

// NewPGQueueConsumer creates a new PostgreSQL-backed queue consumer.
func NewPGQueueConsumer(db *gorm.DB, queueName string, logger *zap.Logger) *PGQueueConsumer {
	if logger == nil {
		logger = zap.L()
	}
	cleanQueue := normalizeQueueName(queueName)
	return &PGQueueConsumer{
		db:        db,
		queueName: cleanQueue,
		logger:    logger,
		pollDelay: 250 * time.Millisecond,
	}
}

// NewPGConsumer alias for NewPGQueueConsumer.
func NewPGConsumer(db *gorm.DB, queueName string, logger *zap.Logger) *PGQueueConsumer {
	return NewPGQueueConsumer(db, queueName, logger)
}

// Start begins consuming in a background goroutine with the given handler.
func (c *PGQueueConsumer) Start(ctx context.Context, handler func(ctx context.Context, body string) error) {
	c.logger.Info("PGQueueConsumer started in background", zap.String("queue", c.queueName))
	go func() {
		_ = c.StartPolling(ctx, handler)
	}()
}

// StartPolling polls for messages synchronously until ctx is canceled.
func (c *PGQueueConsumer) StartPolling(ctx context.Context, handler func(ctx context.Context, body string) error) error {
	c.logger.Info("Starting Postgres queue polling", zap.String("queue", c.queueName))
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Postgres queue consumer stopped", zap.String("queue", c.queueName))
			return nil
		default:
			processed, err := c.pollOnce(ctx, handler)
			if err != nil {
				c.logger.Error("Queue poll error", zap.String("queue", c.queueName), zap.Error(err))
				time.Sleep(1 * time.Second)
			} else if !processed {
				time.Sleep(c.pollDelay)
			}
		}
	}
}

type queueRow struct {
	ID        int64           `gorm:"column:id"`
	QueueName string          `gorm:"column:queue_name"`
	Payload   json.RawMessage `gorm:"column:payload"`
	Attempts  int             `gorm:"column:attempts"`
}

func (c *PGQueueConsumer) pollOnce(ctx context.Context, handler func(ctx context.Context, body string) error) (bool, error) {
	var row queueRow

	tx := c.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return false, tx.Error
	}

	err := tx.Raw(`
		UPDATE message_queues
		SET status = 'processing',
		    attempts = attempts + 1,
		    visible_at = NOW() + INTERVAL '45 seconds',
		    updated_at = NOW()
		WHERE id = (
			SELECT id FROM message_queues
			WHERE queue_name = ?
			  AND status IN ('pending', 'processing')
			  AND visible_at <= NOW()
			  AND attempts < max_attempts
			ORDER BY id ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, queue_name, payload, attempts
	`, c.queueName).Scan(&row).Error

	if err != nil {
		tx.Rollback()
		return false, err
	}

	if row.ID == 0 {
		tx.Rollback()
		return false, nil
	}

	if err := tx.Commit().Error; err != nil {
		return false, err
	}

	// Execute handler outside lock transaction
	handleErr := handler(ctx, string(row.Payload))
	if handleErr == nil {
		_ = c.db.WithContext(ctx).Exec(`DELETE FROM message_queues WHERE id = ?`, row.ID)
	} else {
		c.logger.Warn("Queue message processing failed", zap.Int64("id", row.ID), zap.Error(handleErr))
		_ = c.db.WithContext(ctx).Exec(`
			UPDATE message_queues
			SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'pending' END,
			    error_message = ?,
			    visible_at = NOW() + (attempts * INTERVAL '5 seconds'),
			    updated_at = NOW()
			WHERE id = ?
		`, handleErr.Error(), row.ID)
	}

	return true, nil
}

func resolveQueues(dest string) []string {
	normalized := normalizeQueueName(dest)
	if targets, ok := topicRoutes[normalized]; ok {
		return targets
	}
	return []string{normalized}
}

func normalizeQueueName(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// If it's a URL (e.g. http://localstack:4566/queue/us-east-1/000000000000/notification-queue)
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		if u, err := url.Parse(s); err == nil {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
	}
	// If it's an ARN (e.g. arn:aws:sns:eu-west-2:000000000000:order-events)
	if strings.HasPrefix(s, "arn:aws:") {
		parts := strings.Split(s, ":")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	return s
}
