package services

import (
	"context"
	"fmt"
	"payment-service/models"
	"payment-service/repository"
	"time"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"go.uber.org/zap"
)

type OutboxPublisherConfig struct {
	BatchSize     int
	LeaseDuration time.Duration
	PollInterval  time.Duration
	RetryBase     time.Duration
	RetryMax      time.Duration
}

type OutboxPublisher struct {
	repository repository.OutboxRepository
	sns        aws_pkg.SNSPublisher
	owner      string
	config     OutboxPublisherConfig
}

func NewOutboxPublisher(repo repository.OutboxRepository, sns aws_pkg.SNSPublisher, owner string) *OutboxPublisher {
	return &OutboxPublisher{
		repository: repo,
		sns:        sns,
		owner:      owner,
		config: OutboxPublisherConfig{
			BatchSize:     10,
			LeaseDuration: time.Minute,
			PollInterval:  time.Second,
			RetryBase:     time.Second,
			RetryMax:      time.Minute,
		},
	}
}

// Run polls until ctx is cancelled.
func (p *OutboxPublisher) Run(ctx context.Context) {
	for {
		count, err := p.PublishOnce(ctx)
		if err != nil && ctx.Err() == nil {
			zap.L().Warn("payment outbox publish pass failed", zap.Error(err))
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
	if p.sns == nil {
		return fmt.Errorf("SNS publisher is not configured")
	}
	return p.sns.Publish(ctx, event.Destination, event.Payload)
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
