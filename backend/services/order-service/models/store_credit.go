package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StoreCreditAccount represents the cached balance for a user within a specific store.
// The ledger is the underlying source of truth.
type StoreCreditAccount struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	StoreID         uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_store_credit_accounts_store_user,priority:1" json:"store_id"`
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_store_credit_accounts_store_user,priority:2" json:"user_id"`
	Currency        string    `gorm:"type:varchar(10);not null;default:'GBP'" json:"currency"`
	AvailableAmount int64     `gorm:"not null;default:0" json:"available_amount"`
	HeldAmount      int64     `gorm:"not null;default:0" json:"held_amount"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// BeforeCreate is a GORM lifecycle hook executed before or after database actions.
func (a *StoreCreditAccount) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

// StoreCreditLedgerEntry is an immutable ledger record tracking every balance mutation.
type StoreCreditLedgerEntry struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	AccountID      uuid.UUID  `gorm:"type:uuid;not null;index:idx_store_credit_ledger_account;uniqueIndex:idx_ledger_account_idem_type,priority:1" json:"account_id"`
	EntryType      string     `gorm:"type:varchar(32);not null;uniqueIndex:idx_ledger_account_idem_type,priority:3" json:"entry_type"` // grant, debit, hold, capture, release, refund
	Amount         int64      `gorm:"not null;default:0" json:"amount"`                                                                // net total delta: available_delta + held_delta
	AvailableDelta int64      `gorm:"not null;default:0" json:"available_delta"`
	HeldDelta      int64      `gorm:"not null;default:0" json:"held_delta"`
	Currency       string     `gorm:"type:varchar(10);not null" json:"currency"`
	OrderID        *uuid.UUID `gorm:"type:uuid;index:idx_store_credit_ledger_order" json:"order_id,omitempty"`
	IdempotencyKey string     `gorm:"type:varchar(128);not null;uniqueIndex:idx_ledger_account_idem_type,priority:2" json:"idempotency_key"`
	Reason         string     `gorm:"type:text;not null;default:''" json:"reason"`
	Metadata       []byte     `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedBy      *uuid.UUID `gorm:"type:uuid" json:"created_by,omitempty"`
	CreatedAt      time.Time  `gorm:"index:idx_store_credit_ledger_account,sort:desc" json:"created_at"`
}

// BeforeCreate is a GORM lifecycle hook executed before or after database actions.
func (e *StoreCreditLedgerEntry) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

// OrderPaymentAllocation tracks multi-tender payment breakdown for orders (e.g. Store Credit + Stripe).
type OrderPaymentAllocation struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrderID        uuid.UUID `gorm:"type:uuid;not null;index:idx_order_payment_allocations_order" json:"order_id"`
	PaymentMethod  string    `gorm:"type:varchar(32);not null" json:"payment_method"` // store_credit, stripe
	Amount         int64     `gorm:"not null" json:"amount"`
	Currency       string    `gorm:"type:varchar(10);not null" json:"currency"`
	Status         string    `gorm:"type:varchar(32);not null;default:'allocated'" json:"status"` // allocated, captured, refunded, released
	RefundedAmount int64     `gorm:"not null;default:0" json:"refunded_amount"`
	ReferenceID    string    `gorm:"type:varchar(128);not null;default:''" json:"reference_id"`
	Metadata       []byte    `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// BeforeCreate is a GORM lifecycle hook executed before or after database actions.
func (p *OrderPaymentAllocation) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}
