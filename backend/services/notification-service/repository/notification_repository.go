package repository

import (
	"context"
	"notification-service/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NotificationRepository interface {
	SaveLog(ctx context.Context, log *models.NotificationLog) error
	ClaimEvent(ctx context.Context, eventID string) (bool, error)
	MarkEventDelivered(ctx context.Context, eventID string) error
	ReleaseEvent(ctx context.Context, eventID string) error
	GetLogs(ctx context.Context, filter models.NotificationFilter) ([]models.NotificationLog, int64, error)
	GetLogByID(ctx context.Context, id int64) (*models.NotificationLog, error)
}

func (r *notificationRepository) ClaimEvent(ctx context.Context, eventID string) (bool, error) {
	event := &models.NotificationEvent{EventID: eventID, Status: "processing"}
	tx := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(event)
	return tx.RowsAffected > 0, tx.Error
}

func (r *notificationRepository) MarkEventDelivered(ctx context.Context, eventID string) error {
	return r.db.WithContext(ctx).Model(&models.NotificationEvent{}).
		Where("event_id = ?", eventID).
		Updates(map[string]interface{}{"status": models.NotificationEventDelivered, "processed_at": gorm.Expr("NOW()")}).Error
}

func (r *notificationRepository) ReleaseEvent(ctx context.Context, eventID string) error {
	return r.db.WithContext(ctx).Where("event_id = ?", eventID).Delete(&models.NotificationEvent{}).Error
}

type notificationRepository struct {
	db *gorm.DB
}

func NewNotificationRepository(db *gorm.DB) NotificationRepository {
	return &notificationRepository{db: db}
}

func (r *notificationRepository) SaveLog(ctx context.Context, log *models.NotificationLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

func (r *notificationRepository) GetLogs(ctx context.Context, filter models.NotificationFilter) ([]models.NotificationLog, int64, error) {
	var logs []models.NotificationLog
	var total int64

	if filter.PageSize < 1 {
		filter.PageSize = 10
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	if filter.Page < 1 {
		filter.Page = 1
	}

	query := r.db.WithContext(ctx).Model(&models.NotificationLog{})

	if filter.UserID != "" {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Channel != "" {
		query = query.Where("channel = ?", filter.Channel)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	err := query.Order("created_at DESC").
		Limit(filter.PageSize).
		Offset(offset).
		Find(&logs).Error

	return logs, total, err
}

func (r *notificationRepository) GetLogByID(ctx context.Context, id int64) (*models.NotificationLog, error) {
	var log models.NotificationLog
	err := r.db.WithContext(ctx).First(&log, id).Error
	if err != nil {
		return nil, err
	}
	return &log, nil
}
