package repository

import (
	"context"
	"order-service/payment/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PaymentRepository defines the interface contract for payment repository.
type PaymentRepository interface {
	CreatePayment(ctx context.Context, payment *models.Payment) error
	GetPaymentByOrderID(ctx context.Context, orderID uuid.UUID) (*models.Payment, error)
	GetPaymentByIdempotencyKey(ctx context.Context, key string) (*models.Payment, error)
	GetPaymentByStripeID(ctx context.Context, stripeID string) (*models.Payment, error)
	UpdatePaymentByOrderID(ctx context.Context, orderID uuid.UUID, status string, checkoutURL *string, stripePaymentID *string) error
	Update(ctx context.Context, orderID uuid.UUID, updates map[string]interface{}) error
	// UpdateIfStatusNotIn applies updates only when the current status is not in excludeStatuses,
	// atomically at the SQL level. Returns false (no error) if another writer already claimed
	// the terminal transition — callers must skip side effects (e.g. event publish) in that case.
	UpdateIfStatusNotIn(ctx context.Context, orderID uuid.UUID, excludeStatuses []string, updates map[string]interface{}) (updated bool, err error)
	// UpdateIfStatusNotInWithOutbox does the same atomic conditional update, and — only when it
	// wins the transition — inserts the given outbox events in the same DB transaction, so a
	// status change and its downstream event(s) can never diverge (dual-write).
	UpdateIfStatusNotInWithOutbox(ctx context.Context, orderID uuid.UUID, excludeStatuses []string, updates map[string]interface{}, events []models.OutboxEvent) (updated bool, err error)
	// MarkStripeEventProcessed inserts event_id; returns false if already processed.
	MarkStripeEventProcessed(ctx context.Context, eventID, eventType string) (inserted bool, err error)
	GetUserPaymentMethods(ctx context.Context, userID uuid.UUID) ([]models.UserPaymentMethod, error)
	SaveUserPaymentMethod(ctx context.Context, pm *models.UserPaymentMethod) error
	DeleteUserPaymentMethod(ctx context.Context, userID uuid.UUID, pmID uuid.UUID) error
}

// PaymentRequestClaimer defines the interface contract for payment request claimer.
type PaymentRequestClaimer interface {
	ClaimPaymentRequest(ctx context.Context, payment *models.Payment) (bool, error)
}

type gormPaymentRepo struct {
	db *gorm.DB
}

// NewGormPaymentRepo creates and initializes a new GormPaymentRepo instance.
func NewGormPaymentRepo(db *gorm.DB) PaymentRepository {
	return &gormPaymentRepo{db: db}
}

// CreatePayment performs create payment on r.
func (r *gormPaymentRepo) CreatePayment(ctx context.Context, payment *models.Payment) error {
	return r.db.WithContext(ctx).Create(payment).Error
}

// ClaimPaymentRequest performs claim payment request on r.
func (r *gormPaymentRepo) ClaimPaymentRequest(ctx context.Context, payment *models.Payment) (bool, error) {
	tx := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(payment)
	return tx.RowsAffected > 0, tx.Error
}

// GetPaymentByOrderID returns the payment by order id of the r.
func (r *gormPaymentRepo) GetPaymentByOrderID(ctx context.Context, orderID uuid.UUID) (*models.Payment, error) {
	var payment models.Payment
	if err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&payment).Error; err != nil {
		return nil, err
	}
	return &payment, nil
}

// GetPaymentByIdempotencyKey returns the payment by idempotency key of the r.
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

// GetPaymentByStripeID returns the payment by stripe id of the r.
func (r *gormPaymentRepo) GetPaymentByStripeID(ctx context.Context, stripeID string) (*models.Payment, error) {
	var payment models.Payment
	if err := r.db.WithContext(ctx).Where("stripe_payment_id = ?", stripeID).First(&payment).Error; err != nil {
		return nil, err
	}
	return &payment, nil
}

// UpdatePaymentByOrderID performs update payment by order id on r.
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

// Update performs update on r.
func (r *gormPaymentRepo) Update(ctx context.Context, orderID uuid.UUID, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&models.Payment{}).Where("order_id = ?", orderID).Updates(updates).Error
}

// UpdateIfStatusNotIn performs update if status not in on r.
func (r *gormPaymentRepo) UpdateIfStatusNotIn(ctx context.Context, orderID uuid.UUID, excludeStatuses []string, updates map[string]interface{}) (bool, error) {
	tx := r.db.WithContext(ctx).Model(&models.Payment{}).
		Where("order_id = ? AND status NOT IN ?", orderID, excludeStatuses).
		Updates(updates)
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected > 0, nil
}

// UpdateIfStatusNotInWithOutbox performs update if status not in with outbox on r.
func (r *gormPaymentRepo) UpdateIfStatusNotInWithOutbox(ctx context.Context, orderID uuid.UUID, excludeStatuses []string, updates map[string]interface{}, events []models.OutboxEvent) (bool, error) {
	var updated bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.Payment{}).
			Where("order_id = ? AND status NOT IN ?", orderID, excludeStatuses).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		updated = result.RowsAffected > 0
		if !updated {
			return nil
		}
		for i := range events {
			if err := tx.Create(&events[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return updated, err
}

// MarkStripeEventProcessed performs mark stripe event processed on r.
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

// GetUserPaymentMethods returns the user payment methods of the r.
func (r *gormPaymentRepo) GetUserPaymentMethods(ctx context.Context, userID uuid.UUID) ([]models.UserPaymentMethod, error) {
	var methods []models.UserPaymentMethod
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("is_default DESC, created_at DESC").Find(&methods).Error
	return methods, err
}

// SaveUserPaymentMethod performs save user payment method on r.
func (r *gormPaymentRepo) SaveUserPaymentMethod(ctx context.Context, pm *models.UserPaymentMethod) error {
	return r.db.WithContext(ctx).Save(pm).Error
}

// DeleteUserPaymentMethod performs delete user payment method on r.
func (r *gormPaymentRepo) DeleteUserPaymentMethod(ctx context.Context, userID uuid.UUID, pmID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("id = ? AND user_id = ?", pmID, userID).Delete(&models.UserPaymentMethod{}).Error
}
