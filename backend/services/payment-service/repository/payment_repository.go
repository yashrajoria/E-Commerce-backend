package repository

import (
	"context"
	"payment-service/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PaymentRepository interface {
	CreatePayment(ctx context.Context, payment *models.Payment) error
	GetPaymentByOrderID(ctx context.Context, orderID uuid.UUID) (*models.Payment, error)
	GetPaymentByIdempotencyKey(ctx context.Context, key string) (*models.Payment, error)
	GetPaymentByStripeID(ctx context.Context, stripeID string) (*models.Payment, error)
	UpdatePaymentByOrderID(ctx context.Context, orderID uuid.UUID, status string, checkoutURL *string, stripePaymentID *string) error
	Update(ctx context.Context, orderID uuid.UUID, updates map[string]interface{}) error
	// MarkStripeEventProcessed inserts event_id; returns false if already processed.
	MarkStripeEventProcessed(ctx context.Context, eventID, eventType string) (inserted bool, err error)
}

type gormPaymentRepo struct {
	db *gorm.DB
}

func NewGormPaymentRepo(db *gorm.DB) PaymentRepository {
	return &gormPaymentRepo{db: db}
}

func (r *gormPaymentRepo) CreatePayment(ctx context.Context, payment *models.Payment) error {
	return r.db.WithContext(ctx).Create(payment).Error
}

func (r *gormPaymentRepo) GetPaymentByOrderID(ctx context.Context, orderID uuid.UUID) (*models.Payment, error) {
	var payment models.Payment
	if err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&payment).Error; err != nil {
		return nil, err
	}
	return &payment, nil
}

func (r *gormPaymentRepo) GetPaymentByIdempotencyKey(ctx context.Context, key string) (*models.Payment, error) {
	var payment models.Payment
	if err := r.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&payment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // Return nil if no record is found
		}
		return nil, err // Return the error for other cases
	}
	return &payment, nil
}

func (r *gormPaymentRepo) GetPaymentByStripeID(ctx context.Context, stripeID string) (*models.Payment, error) {
	var payment models.Payment
	if err := r.db.WithContext(ctx).Where("stripe_payment_id = ?", stripeID).First(&payment).Error; err != nil {
		return nil, err
	}
	return &payment, nil
}

func (r *gormPaymentRepo) UpdatePaymentByOrderID(ctx context.Context, orderID uuid.UUID, status string, checkoutURL *string, stripePaymentID *string) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if checkoutURL != nil {
		updates["checkout_url"] = checkoutURL
	}
	if stripePaymentID != nil {
		updates["stripe_payment_id"] = stripePaymentID
	}
	return r.Update(ctx, orderID, updates)
}

func (r *gormPaymentRepo) Update(ctx context.Context, orderID uuid.UUID, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&models.Payment{}).Where("order_id = ?", orderID).Updates(updates).Error
}

func (r *gormPaymentRepo) MarkStripeEventProcessed(ctx context.Context, eventID, eventType string) (bool, error) {
	tx := r.db.WithContext(ctx).Exec(
		`INSERT INTO stripe_processed_events (event_id, event_type, processed_at) VALUES (?, ?, NOW()) ON CONFLICT (event_id) DO NOTHING`,
		eventID, eventType,
	)
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected > 0, nil
}
