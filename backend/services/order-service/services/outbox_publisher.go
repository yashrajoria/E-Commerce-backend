package services

import (
	"context"
	"fmt"
	"order-service/models"
	repositories "order-service/repository"
	"time"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"go.uber.org/zap"
)

type OutboxSQSPublisher interface {
	Publish(ctx context.Context, destination string, message []byte) error
}

type OutboxPublisherConfig struct {
	BatchSize     int
	LeaseDuration time.Duration
	PollInterval  time.Duration
	RetryBase     time.Duration
	RetryMax      time.Duration
}

type OutboxPublisher struct {
	repository repositories.OutboxRepository
	sqs        OutboxSQSPublisher
	sns        aws_pkg.SNSPublisher
	owner      string
	config     OutboxPublisherConfig
}

func NewOutboxPublisher(repository repositories.OutboxRepository, sqs OutboxSQSPublisher, sns aws_pkg.SNSPublisher, owner string) *OutboxPublisher {
	return NewOutboxPublisherWithConfig(repository, sqs, sns, owner, OutboxPublisherConfig{
		BatchSize:     10,
		LeaseDuration: time.Minute,
		PollInterval:  time.Second,
		RetryBase:     time.Second,
		RetryMax:      time.Minute,
	})
}

func NewOutboxPublisherWithConfig(repository repositories.OutboxRepository, sqs OutboxSQSPublisher, sns aws_pkg.SNSPublisher, owner string, config OutboxPublisherConfig) *OutboxPublisher {
	if config.BatchSize <= 0 {
		config.BatchSize = 10
	}
	if config.LeaseDuration <= 0 {
		config.LeaseDuration = time.Minute
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.RetryBase <= 0 {
		config.RetryBase = time.Second
	}
	if config.RetryMax < config.RetryBase {
		config.RetryMax = config.RetryBase
	}
	return &OutboxPublisher{repository: repository, sqs: sqs, sns: sns, owner: owner, config: config}
}

// Run polls until ctx is cancelled. The timer is always cancellable for graceful shutdown.
func (p *OutboxPublisher) Run(ctx context.Context) {
	for {
		count, err := p.PublishOnce(ctx)
		if err != nil && ctx.Err() == nil {
			zap.L().Warn("outbox publish pass failed", zap.Error(err))
		}
		if ctx.Err() != nil {
			return
		}
		if count > 0 {
			continue
		}
		timer := time.NewTimer(p.config.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (p *OutboxPublisher) PublishOnce(ctx context.Context) (int, error) {
	events, err := p.repository.ClaimWithLease(ctx, p.config.BatchSize, p.owner, p.config.LeaseDuration)
	if err != nil {
		return 0, err
	}
	for _, event := range events {
		if err := p.publish(ctx, event); err != nil {
			availableAt := time.Now().UTC().Add(p.retryDelay(event.Attempts))
			if markErr := p.repository.MarkFailedByOwner(ctx, event.ID, p.owner, err.Error(), availableAt); markErr != nil {
				return len(events), fmt.Errorf("publish %s: %v; mark failed: %w", event.ID, err, markErr)
			}
			return len(events), err
		}
		if err := p.repository.MarkPublishedByOwner(ctx, event.ID, p.owner); err != nil {
			return len(events), fmt.Errorf("mark event %s published: %w", event.ID, err)
		}
	}
	return len(events), nil
}

func (p *OutboxPublisher) publish(ctx context.Context, event models.OutboxEvent) error {
	switch event.DestinationType {
	case models.OutboxDestinationSQS:
		if p.sqs == nil {
			return fmt.Errorf("SQS publisher is not configured")
		}
		return p.sqs.Publish(ctx, event.Destination, event.Payload)
	case models.OutboxDestinationSNS:
		if p.sns == nil {
			return fmt.Errorf("SNS publisher is not configured")
		}
		return p.sns.Publish(ctx, event.Destination, event.Payload)
	default:
		return fmt.Errorf("unsupported outbox destination %q", event.DestinationType)
	}
}

func (p *OutboxPublisher) retryDelay(attempts int) time.Duration {
	delay := p.config.RetryBase
	for index := 1; index < attempts && delay < p.config.RetryMax; index++ {
		delay *= 2
	}
	if delay > p.config.RetryMax {
		return p.config.RetryMax
	}
	return delay
}
