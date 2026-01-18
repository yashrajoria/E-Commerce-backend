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
	UpdatePaymentByOrderID(ctx context.Context, orderID uuid.UUID, status string, checkoutURL *string, stripePaymentID *string) error
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

func (r *gormPaymentRepo) UpdatePaymentByOrderID(ctx context.Context, orderID uuid.UUID, status string, checkoutURL *string, stripePaymentID *string) error {
	updates := map[string]interface{}{
		"status":       status,
		"checkout_url": checkoutURL,
	}
	if stripePaymentID != nil {
		updates["stripe_payment_id"] = stripePaymentID
	}
	return r.db.WithContext(ctx).Model(&models.Payment{}).Where("order_id = ?", orderID).Updates(updates).Error
}
