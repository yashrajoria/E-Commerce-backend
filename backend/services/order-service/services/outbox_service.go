package services

import (
	"context"
	"order-service/models"
	repositories "order-service/repository"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OutboxService interface {
	Enqueue(ctx context.Context, tx *gorm.DB, event *models.OutboxEvent) error
	Claim(ctx context.Context, limit int) ([]models.OutboxEvent, error)
	ClaimWithLease(ctx context.Context, limit int, owner string, leaseDuration time.Duration) ([]models.OutboxEvent, error)
	MarkPublished(ctx context.Context, eventID uuid.UUID) error
	MarkPublishedByOwner(ctx context.Context, eventID uuid.UUID, owner string) error
	MarkFailed(ctx context.Context, eventID uuid.UUID, eventErr string, availableAt time.Time) error
	MarkFailedByOwner(ctx context.Context, eventID uuid.UUID, owner, eventErr string, availableAt time.Time) error
}

type GormOutboxService struct {
	repository repositories.OutboxRepository
}

func NewOutboxService(repository repositories.OutboxRepository) OutboxService {
	return &GormOutboxService{repository: repository}
}

func (s *GormOutboxService) Enqueue(ctx context.Context, tx *gorm.DB, event *models.OutboxEvent) error {
	return s.repository.Create(ctx, tx, event)
}

func (s *GormOutboxService) Claim(ctx context.Context, limit int) ([]models.OutboxEvent, error) {
	return s.repository.Claim(ctx, limit)
}

func (s *GormOutboxService) ClaimWithLease(ctx context.Context, limit int, owner string, leaseDuration time.Duration) ([]models.OutboxEvent, error) {
	return s.repository.ClaimWithLease(ctx, limit, owner, leaseDuration)
}

func (s *GormOutboxService) MarkPublished(ctx context.Context, eventID uuid.UUID) error {
	return s.repository.MarkPublished(ctx, eventID)
}

func (s *GormOutboxService) MarkPublishedByOwner(ctx context.Context, eventID uuid.UUID, owner string) error {
	return s.repository.MarkPublishedByOwner(ctx, eventID, owner)
}

func (s *GormOutboxService) MarkFailed(ctx context.Context, eventID uuid.UUID, eventErr string, availableAt time.Time) error {
	return s.repository.MarkFailed(ctx, eventID, eventErr, availableAt)
}

func (s *GormOutboxService) MarkFailedByOwner(ctx context.Context, eventID uuid.UUID, owner, eventErr string, availableAt time.Time) error {
	return s.repository.MarkFailedByOwner(ctx, eventID, owner, eventErr, availableAt)
}
