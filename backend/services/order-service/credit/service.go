// Package credit provides HTTP handlers and ledger services for store credit accounts and balances.
package credit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yashrajoria/common/messaging"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"order-service/models"
)

var (
	ErrInsufficientCredit = errors.New("insufficient store credit")
	ErrInsufficientHold   = errors.New("insufficient held store credit to capture or release")
	ErrInvalidAmount      = errors.New("amount must be greater than zero")
	ErrMissingIdempotency = errors.New("idempotency key is required")
)

// Service represents service.
type Service struct {
	db        *gorm.DB
	publisher messaging.Publisher
	topic     string
}

// NewService creates and initializes a new Service instance.
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// SetEventPublisher executes business logic for set event publisher.
func (s *Service) SetEventPublisher(p messaging.Publisher, topic string) {
	s.publisher, s.topic = p, topic
}

func (s *Service) lockOrCreateAccount(tx *gorm.DB, storeID, userID uuid.UUID, currency string) (*models.StoreCreditAccount, error) {
	if currency == "" {
		currency = "GBP"
	}
	var a models.StoreCreditAccount
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("store_id = ? AND user_id = ?", storeID, userID).
		First(&a).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		a = models.StoreCreditAccount{
			StoreID:         storeID,
			UserID:          userID,
			Currency:        currency,
			AvailableAmount: 0,
			HeldAmount:      0,
			CreatedAt:       time.Now().UTC(),
			UpdatedAt:       time.Now().UTC(),
		}
		if err := tx.Create(&a).Error; err != nil {
			// In case of concurrent creation race, retry locking
			if lockErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("store_id = ? AND user_id = ?", storeID, userID).
				First(&a).Error; lockErr == nil {
				return &a, nil
			}
			return nil, err
		}
		return &a, nil
	}
	return &a, err
}

// Account executes business logic for account.
func (s *Service) Account(ctx context.Context, storeID, userID uuid.UUID) (*models.StoreCreditAccount, error) {
	var a models.StoreCreditAccount
	err := s.db.WithContext(ctx).Where("store_id = ? AND user_id = ?", storeID, userID).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &models.StoreCreditAccount{
			StoreID:         storeID,
			UserID:          userID,
			Currency:        "GBP",
			AvailableAmount: 0,
			HeldAmount:      0,
		}, nil
	}
	return &a, err
}

// Balance executes business logic for balance.
func (s *Service) Balance(ctx context.Context, storeID, userID uuid.UUID) (int64, int64, string, error) {
	a, err := s.Account(ctx, storeID, userID)
	if err != nil {
		return 0, 0, "", err
	}
	return a.AvailableAmount, a.HeldAmount, a.Currency, nil
}

// Grant executes business logic for grant.
func (s *Service) Grant(ctx context.Context, storeID, userID uuid.UUID, amount int64, currency, reason, idem string, actor *uuid.UUID, orderID *uuid.UUID) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	if strings.TrimSpace(idem) == "" {
		return ErrMissingIdempotency
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := s.lockOrCreateAccount(tx, storeID, userID, currency)
		if err != nil {
			return err
		}

		var existing models.StoreCreditLedgerEntry
		if err := tx.Where("account_id = ? AND idempotency_key = ? AND entry_type = ?", a.ID, idem, "grant").First(&existing).Error; err == nil {
			return nil // idempotent duplicate
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		a.AvailableAmount += amount
		a.UpdatedAt = time.Now().UTC()
		if err := tx.Save(a).Error; err != nil {
			return err
		}

		entry := models.StoreCreditLedgerEntry{
			AccountID:      a.ID,
			EntryType:      "grant",
			Amount:         amount,
			AvailableDelta: amount,
			HeldDelta:      0,
			Currency:       a.Currency,
			OrderID:        orderID,
			IdempotencyKey: idem,
			Reason:         reason,
			Metadata:       []byte(`{}`),
			CreatedBy:      actor,
			CreatedAt:      time.Now().UTC(),
		}
		return tx.Create(&entry).Error
	})

	if err == nil {
		s.emit(ctx, "store_credit.granted", storeID, userID, amount, currency, orderID, idem)
	}
	return err
}

// Debit executes business logic for debit.
func (s *Service) Debit(ctx context.Context, storeID, userID uuid.UUID, amount int64, currency, reason, idem string, actor *uuid.UUID) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	if strings.TrimSpace(idem) == "" {
		return ErrMissingIdempotency
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := s.lockOrCreateAccount(tx, storeID, userID, currency)
		if err != nil {
			return err
		}

		var existing models.StoreCreditLedgerEntry
		if err := tx.Where("account_id = ? AND idempotency_key = ? AND entry_type = ?", a.ID, idem, "debit").First(&existing).Error; err == nil {
			return nil // idempotent duplicate
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if a.AvailableAmount < amount {
			return ErrInsufficientCredit
		}

		a.AvailableAmount -= amount
		a.UpdatedAt = time.Now().UTC()
		if err := tx.Save(a).Error; err != nil {
			return err
		}

		entry := models.StoreCreditLedgerEntry{
			AccountID:      a.ID,
			EntryType:      "debit",
			Amount:         -amount,
			AvailableDelta: -amount,
			HeldDelta:      0,
			Currency:       a.Currency,
			IdempotencyKey: idem,
			Reason:         reason,
			Metadata:       []byte(`{}`),
			CreatedBy:      actor,
			CreatedAt:      time.Now().UTC(),
		}
		return tx.Create(&entry).Error
	})

	if err == nil {
		s.emit(ctx, "store_credit.debited", storeID, userID, amount, currency, nil, idem)
	}
	return err
}

// Hold executes business logic for hold.
func (s *Service) Hold(ctx context.Context, storeID, userID uuid.UUID, amount int64, currency, idem string, orderID *uuid.UUID) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	if strings.TrimSpace(idem) == "" {
		return ErrMissingIdempotency
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := s.lockOrCreateAccount(tx, storeID, userID, currency)
		if err != nil {
			return err
		}

		var existing models.StoreCreditLedgerEntry
		if err := tx.Where("account_id = ? AND idempotency_key = ? AND entry_type = ?", a.ID, idem, "hold").First(&existing).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if a.AvailableAmount < amount {
			return ErrInsufficientCredit
		}

		a.AvailableAmount -= amount
		a.HeldAmount += amount
		a.UpdatedAt = time.Now().UTC()
		if err := tx.Save(a).Error; err != nil {
			return err
		}

		entry := models.StoreCreditLedgerEntry{
			AccountID:      a.ID,
			EntryType:      "hold",
			Amount:         0, // Total customer funds untouched, shifted available -> held
			AvailableDelta: -amount,
			HeldDelta:      amount,
			Currency:       a.Currency,
			OrderID:        orderID,
			IdempotencyKey: idem,
			Reason:         "checkout_reservation",
			Metadata:       []byte(`{}`),
			CreatedAt:      time.Now().UTC(),
		}
		return tx.Create(&entry).Error
	})

	if err == nil {
		s.emit(ctx, "store_credit.held", storeID, userID, amount, currency, orderID, idem)
	}
	return err
}

// Capture executes business logic for capture.
func (s *Service) Capture(ctx context.Context, storeID, userID uuid.UUID, amount int64, currency, idem string, orderID *uuid.UUID) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	if strings.TrimSpace(idem) == "" {
		return ErrMissingIdempotency
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := s.lockOrCreateAccount(tx, storeID, userID, currency)
		if err != nil {
			return err
		}

		var existing models.StoreCreditLedgerEntry
		if err := tx.Where("account_id = ? AND idempotency_key = ? AND entry_type = ?", a.ID, idem, "capture").First(&existing).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if a.HeldAmount < amount {
			return ErrInsufficientHold
		}

		a.HeldAmount -= amount
		a.UpdatedAt = time.Now().UTC()
		if err := tx.Save(a).Error; err != nil {
			return err
		}

		entry := models.StoreCreditLedgerEntry{
			AccountID:      a.ID,
			EntryType:      "capture",
			Amount:         -amount, // Funds leave the customer
			AvailableDelta: 0,
			HeldDelta:      -amount,
			Currency:       a.Currency,
			OrderID:        orderID,
			IdempotencyKey: idem,
			Reason:         "order_completed",
			Metadata:       []byte(`{}`),
			CreatedAt:      time.Now().UTC(),
		}
		return tx.Create(&entry).Error
	})

	if err == nil {
		s.emit(ctx, "store_credit.captured", storeID, userID, amount, currency, orderID, idem)
	}
	return err
}

// Release executes business logic for release.
func (s *Service) Release(ctx context.Context, storeID, userID uuid.UUID, amount int64, currency, idem string, orderID *uuid.UUID) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	if strings.TrimSpace(idem) == "" {
		return ErrMissingIdempotency
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := s.lockOrCreateAccount(tx, storeID, userID, currency)
		if err != nil {
			return err
		}

		var existing models.StoreCreditLedgerEntry
		if err := tx.Where("account_id = ? AND idempotency_key = ? AND entry_type = ?", a.ID, idem, "release").First(&existing).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if a.HeldAmount < amount {
			return ErrInsufficientHold
		}

		a.HeldAmount -= amount
		a.AvailableAmount += amount
		a.UpdatedAt = time.Now().UTC()
		if err := tx.Save(a).Error; err != nil {
			return err
		}

		entry := models.StoreCreditLedgerEntry{
			AccountID:      a.ID,
			EntryType:      "release",
			Amount:         0, // Funds restored to available
			AvailableDelta: amount,
			HeldDelta:      -amount,
			Currency:       a.Currency,
			OrderID:        orderID,
			IdempotencyKey: idem,
			Reason:         "checkout_canceled_or_expired",
			Metadata:       []byte(`{}`),
			CreatedAt:      time.Now().UTC(),
		}
		return tx.Create(&entry).Error
	})

	if err == nil {
		s.emit(ctx, "store_credit.released", storeID, userID, amount, currency, orderID, idem)
	}
	return err
}

// Refund executes business logic for refund.
func (s *Service) Refund(ctx context.Context, storeID, userID uuid.UUID, amount int64, currency, reason, idem string, actor *uuid.UUID, orderID *uuid.UUID) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	if strings.TrimSpace(idem) == "" {
		return ErrMissingIdempotency
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := s.lockOrCreateAccount(tx, storeID, userID, currency)
		if err != nil {
			return err
		}

		var existing models.StoreCreditLedgerEntry
		if err := tx.Where("account_id = ? AND idempotency_key = ? AND entry_type = ?", a.ID, idem, "refund").First(&existing).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		a.AvailableAmount += amount
		a.UpdatedAt = time.Now().UTC()
		if err := tx.Save(a).Error; err != nil {
			return err
		}

		entry := models.StoreCreditLedgerEntry{
			AccountID:      a.ID,
			EntryType:      "refund",
			Amount:         amount,
			AvailableDelta: amount,
			HeldDelta:      0,
			Currency:       a.Currency,
			OrderID:        orderID,
			IdempotencyKey: idem,
			Reason:         reason,
			Metadata:       []byte(`{}`),
			CreatedBy:      actor,
			CreatedAt:      time.Now().UTC(),
		}
		return tx.Create(&entry).Error
	})

	if err == nil {
		s.emit(ctx, "store_credit.refunded", storeID, userID, amount, currency, orderID, idem)
	}
	return err
}

// Ledger executes business logic for ledger.
func (s *Service) Ledger(ctx context.Context, storeID, userID uuid.UUID, limit int) ([]models.StoreCreditLedgerEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	a, err := s.Account(ctx, storeID, userID)
	if err != nil {
		return nil, err
	}
	if a.ID == uuid.Nil {
		return []models.StoreCreditLedgerEntry{}, nil
	}
	var entries []models.StoreCreditLedgerEntry
	err = s.db.WithContext(ctx).Where("account_id = ?", a.ID).
		Order("created_at DESC").
		Limit(limit).
		Find(&entries).Error
	return entries, err
}

// AuditReconcile verifies that the cached balances in store_credit_accounts strictly match the ledger.
func (s *Service) AuditReconcile(ctx context.Context, storeID, userID uuid.UUID) (expectedAvail int64, expectedHeld int64, actualAvail int64, actualHeld int64, matches bool, err error) {
	a, err := s.Account(ctx, storeID, userID)
	if err != nil {
		return 0, 0, 0, 0, false, err
	}
	if a.ID == uuid.Nil {
		return 0, 0, 0, 0, true, nil
	}

	type SumResult struct {
		SumAvail int64 `gorm:"column:sum_avail"`
		SumHeld  int64 `gorm:"column:sum_held"`
	}
	var res SumResult
	err = s.db.WithContext(ctx).Model(&models.StoreCreditLedgerEntry{}).
		Where("account_id = ?", a.ID).
		Select("COALESCE(SUM(available_delta), 0) AS sum_avail, COALESCE(SUM(held_delta), 0) AS sum_held").
		Scan(&res).Error
	if err != nil {
		return 0, 0, 0, 0, false, err
	}

	expectedAvail = res.SumAvail
	expectedHeld = res.SumHeld
	actualAvail = a.AvailableAmount
	actualHeld = a.HeldAmount
	matches = (expectedAvail == actualAvail) && (expectedHeld == actualHeld)
	return expectedAvail, expectedHeld, actualAvail, actualHeld, matches, nil
}

func (s *Service) emit(ctx context.Context, eventType string, storeID, userID uuid.UUID, amount int64, currency string, orderID *uuid.UUID, idem string) {
	if s.publisher == nil || s.topic == "" {
		return
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"event_type":      eventType,
		"store_id":        storeID.String(),
		"user_id":         userID.String(),
		"amount":          amount,
		"currency":        currency,
		"order_id":        orderID,
		"idempotency_key": idem,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	})
	_ = s.publisher.Publish(ctx, s.topic, payload)
}
