package repositories

import (
	"context"
	"errors"
	"order-service/models"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInvalidClaimLimit = errors.New("outbox claim limit must be greater than zero")
var ErrInvalidLease = errors.New("outbox lease owner and duration are required")
var ErrOutboxLeaseLost = errors.New("outbox lease was lost")

type OutboxRepository interface {
	Create(ctx context.Context, tx *gorm.DB, event *models.OutboxEvent) error
	Claim(ctx context.Context, limit int) ([]models.OutboxEvent, error)
	ClaimWithLease(ctx context.Context, limit int, owner string, leaseDuration time.Duration) ([]models.OutboxEvent, error)
	MarkPublished(ctx context.Context, eventID uuid.UUID) error
	MarkPublishedByOwner(ctx context.Context, eventID uuid.UUID, owner string) error
	MarkFailed(ctx context.Context, eventID uuid.UUID, eventErr string, availableAt time.Time) error
	MarkFailedByOwner(ctx context.Context, eventID uuid.UUID, owner, eventErr string, availableAt time.Time) error
}

type GormOutboxRepository struct {
	db *gorm.DB
}

func NewGormOutboxRepository(db *gorm.DB) OutboxRepository {
	return &GormOutboxRepository{db: db}
}

func (r *GormOutboxRepository) Create(ctx context.Context, tx *gorm.DB, event *models.OutboxEvent) error {
	if tx == nil {
		tx = r.db
	}
	return tx.WithContext(ctx).Create(event).Error
}

func (r *GormOutboxRepository) Claim(ctx context.Context, limit int) ([]models.OutboxEvent, error) {
	return r.ClaimWithLease(ctx, limit, uuid.NewString(), time.Minute)
}

func (r *GormOutboxRepository) ClaimWithLease(ctx context.Context, limit int, owner string, leaseDuration time.Duration) ([]models.OutboxEvent, error) {
	if limit <= 0 {
		return nil, ErrInvalidClaimLimit
	}
	if owner == "" || leaseDuration <= 0 {
		return nil, ErrInvalidLease
	}

	var events []models.OutboxEvent
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("available_at <= ? AND (status = ? OR (status = ? AND lease_expires_at <= ?))", now, models.OutboxStatusPending, models.OutboxStatusProcessing, now).
			Order("created_at ASC").
			Limit(limit).
			Find(&events).Error; err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}

		ids := make([]uuid.UUID, 0, len(events))
		for _, event := range events {
			ids = append(ids, event.ID)
		}
		return tx.Model(&models.OutboxEvent{}).
			Where("id IN ? AND (status = ? OR (status = ? AND lease_expires_at <= ?))", ids, models.OutboxStatusPending, models.OutboxStatusProcessing, now).
			Updates(map[string]interface{}{
				"status":           models.OutboxStatusProcessing,
				"attempts":         gorm.Expr("attempts + 1"),
				"lease_owner":      owner,
				"lease_expires_at": now.Add(leaseDuration),
				"claimed_at":       now,
				"updated_at":       now,
			}).Error
	})
	if err == nil {
		claimedAt := time.Now().UTC()
		leaseExpiresAt := claimedAt.Add(leaseDuration)
		for index := range events {
			events[index].Status = models.OutboxStatusProcessing
			events[index].Attempts++
			events[index].LeaseOwner = &owner
			events[index].LeaseExpiresAt = &leaseExpiresAt
			events[index].ClaimedAt = &claimedAt
		}
	}
	return events, err
}

func (r *GormOutboxRepository) MarkPublished(ctx context.Context, eventID uuid.UUID) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&models.OutboxEvent{}).
		Where("id = ? AND status = ?", eventID, models.OutboxStatusProcessing).
		Updates(map[string]interface{}{
			"status":       models.OutboxStatusPublished,
			"published_at": now,
			"updated_at":   now,
		}).Error
}

func (r *GormOutboxRepository) MarkPublishedByOwner(ctx context.Context, eventID uuid.UUID, owner string) error {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&models.OutboxEvent{}).
		Where("id = ? AND status = ? AND lease_owner = ?", eventID, models.OutboxStatusProcessing, owner).
		Updates(map[string]interface{}{
			"status":           models.OutboxStatusPublished,
			"published_at":     now,
			"lease_owner":      nil,
			"lease_expires_at": nil,
			"updated_at":       now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrOutboxLeaseLost
	}
	return nil
}

func (r *GormOutboxRepository) MarkFailed(ctx context.Context, eventID uuid.UUID, eventErr string, availableAt time.Time) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&models.OutboxEvent{}).
		Where("id = ? AND status = ?", eventID, models.OutboxStatusProcessing).
		Updates(map[string]interface{}{
			"status":           models.OutboxStatusPending,
			"last_error":       eventErr,
			"available_at":     availableAt,
			"lease_owner":      nil,
			"lease_expires_at": nil,
			"updated_at":       now,
		}).Error
}

func (r *GormOutboxRepository) MarkFailedByOwner(ctx context.Context, eventID uuid.UUID, owner, eventErr string, availableAt time.Time) error {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&models.OutboxEvent{}).
		Where("id = ? AND status = ? AND lease_owner = ?", eventID, models.OutboxStatusProcessing, owner).
		Updates(map[string]interface{}{
			"status":           models.OutboxStatusPending,
			"last_error":       eventErr,
			"available_at":     availableAt,
			"lease_owner":      nil,
			"lease_expires_at": nil,
			"updated_at":       now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrOutboxLeaseLost
	}
	return nil
}
