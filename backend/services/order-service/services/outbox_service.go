package services

import (
	"context"
	"order-service/models"
	repositories "order-service/repository"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OutboxService defines the interface contract for outbox service.
type OutboxService interface {
	Enqueue(ctx context.Context, tx *gorm.DB, event *models.OutboxEvent) error
	Claim(ctx context.Context, limit int) ([]models.OutboxEvent, error)
	ClaimWithLease(ctx context.Context, limit int, owner string, leaseDuration time.Duration) ([]models.OutboxEvent, error)
	MarkPublished(ctx context.Context, eventID uuid.UUID) error
	MarkPublishedByOwner(ctx context.Context, eventID uuid.UUID, owner string) error
	MarkFailed(ctx context.Context, eventID uuid.UUID, eventErr string, availableAt time.Time) error
	MarkFailedByOwner(ctx context.Context, eventID uuid.UUID, owner, eventErr string, availableAt time.Time) error
}

// GormOutboxService represents gorm outbox service.
type GormOutboxService struct {
	repository repositories.OutboxRepository
}

// NewOutboxService creates and initializes a new OutboxService instance.
func NewOutboxService(repository repositories.OutboxRepository) OutboxService {
	return &GormOutboxService{repository: repository}
}

// Enqueue executes business logic for enqueue.
func (s *GormOutboxService) Enqueue(ctx context.Context, tx *gorm.DB, event *models.OutboxEvent) error {
	return s.repository.Create(ctx, tx, event)
}

// Claim executes business logic for claim.
func (s *GormOutboxService) Claim(ctx context.Context, limit int) ([]models.OutboxEvent, error) {
	return s.repository.Claim(ctx, limit)
}

// ClaimWithLease executes business logic for claim with lease.
func (s *GormOutboxService) ClaimWithLease(ctx context.Context, limit int, owner string, leaseDuration time.Duration) ([]models.OutboxEvent, error) {
	return s.repository.ClaimWithLease(ctx, limit, owner, leaseDuration)
}

// MarkPublished executes business logic for mark published.
func (s *GormOutboxService) MarkPublished(ctx context.Context, eventID uuid.UUID) error {
	return s.repository.MarkPublished(ctx, eventID)
}

// MarkPublishedByOwner executes business logic for mark published by owner.
func (s *GormOutboxService) MarkPublishedByOwner(ctx context.Context, eventID uuid.UUID, owner string) error {
	return s.repository.MarkPublishedByOwner(ctx, eventID, owner)
}

// MarkFailed executes business logic for mark failed.
func (s *GormOutboxService) MarkFailed(ctx context.Context, eventID uuid.UUID, eventErr string, availableAt time.Time) error {
	return s.repository.MarkFailed(ctx, eventID, eventErr, availableAt)
}

// MarkFailedByOwner executes business logic for mark failed by owner.
func (s *GormOutboxService) MarkFailedByOwner(ctx context.Context, eventID uuid.UUID, owner, eventErr string, availableAt time.Time) error {
	return s.repository.MarkFailedByOwner(ctx, eventID, owner, eventErr, availableAt)
}
