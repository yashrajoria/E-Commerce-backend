package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StoreRole defines authorization role classifications.
type StoreRole string

const (
	StoreRoleOwner   StoreRole = "owner"
	StoreRoleAdmin   StoreRole = "admin"
	StoreRoleManager StoreRole = "manager"
	StoreRoleViewer  StoreRole = "viewer"
)

// Store represents store.
type Store struct {
	ID          uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name        string         `gorm:"size:100;not null" json:"name"`
	Slug        string         `gorm:"size:100;not null;uniqueIndex:idx_stores_slug,where:deleted_at IS NULL" json:"slug"`
	Description *string        `gorm:"type:text" json:"description,omitempty"`
	LogoURL     *string        `gorm:"type:text" json:"logo_url,omitempty"`
	Currency    string         `gorm:"size:10;not null;default:'GBP'" json:"currency"`
	Timezone    string         `gorm:"size:50;not null;default:'Europe/London'" json:"timezone"`
	IsActive    bool           `gorm:"not null;default:true" json:"is_active"`
	CreatedAt   time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	Memberships  []StoreMembership  `gorm:"foreignKey:StoreID" json:"memberships,omitempty"`
	Settings     *StoreSettings     `gorm:"foreignKey:StoreID" json:"settings,omitempty"`
	Subscription *StoreSubscription `gorm:"foreignKey:StoreID" json:"subscription,omitempty"`
}

// StoreMembership represents store membership.
type StoreMembership struct {
	ID        uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	StoreID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"store_id"`
	Role      StoreRole  `gorm:"type:store_role;not null;default:'viewer'" json:"role"`
	InvitedBy *uuid.UUID `gorm:"type:uuid" json:"invited_by,omitempty"`
	Accepted  bool       `gorm:"not null;default:false" json:"accepted"`
	CreatedAt time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	User  *User  `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Store *Store `gorm:"foreignKey:StoreID" json:"store,omitempty"`
}

// StoreSettings represents store settings.
type StoreSettings struct {
	StoreID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"store_id"`
	ContactEmail     *string   `gorm:"type:text" json:"contact_email,omitempty"`
	ContactPhone     *string   `gorm:"type:text" json:"contact_phone,omitempty"`
	Address          *string   `gorm:"type:text" json:"address,omitempty"`
	SMTPHost         *string   `gorm:"type:text" json:"smtp_host,omitempty"`
	SMTPPort         *int      `json:"smtp_port,omitempty"`
	MaintenanceMode  bool      `gorm:"not null;default:false" json:"maintenance_mode"`
	AnalyticsEnabled bool      `gorm:"not null;default:true" json:"analytics_enabled"`
	ShippingConfig   string    `gorm:"type:jsonb;default:'{}'" json:"shipping_config"`
	PaymentConfig    string    `gorm:"type:jsonb;default:'{}'" json:"payment_config"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// Plan represents plan.
type Plan struct {
	ID           uuid.UUID         `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name         string            `gorm:"size:50;not null" json:"name"`
	Slug         string            `gorm:"size:50;not null;unique" json:"slug"`
	PriceMonthly int               `gorm:"not null;default:0" json:"price_monthly"`
	PriceYearly  int               `gorm:"not null;default:0" json:"price_yearly"`
	Currency     string            `gorm:"size:10;not null;default:'GBP'" json:"currency"`
	IsActive     bool              `gorm:"not null;default:true" json:"is_active"`
	CreatedAt    time.Time         `gorm:"autoCreateTime" json:"created_at"`
	Entitlements []PlanEntitlement `gorm:"foreignKey:PlanID" json:"entitlements,omitempty"`
}

// PlanEntitlement represents plan entitlement.
type PlanEntitlement struct {
	ID         uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	PlanID     uuid.UUID `gorm:"type:uuid;not null;index" json:"plan_id"`
	FeatureKey string    `gorm:"size:100;not null" json:"feature_key"`
	LimitValue *int      `json:"limit_value"`
	Enabled    bool      `gorm:"not null;default:true" json:"enabled"`
}

// StoreSubscription represents store subscription.
type StoreSubscription struct {
	ID                   uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	StoreID              uuid.UUID  `gorm:"type:uuid;not null;index" json:"store_id"`
	PlanID               uuid.UUID  `gorm:"type:uuid;not null;index" json:"plan_id"`
	Status               string     `gorm:"size:30;not null;default:'active'" json:"status"`
	StripeSubscriptionID *string    `gorm:"type:text" json:"stripe_subscription_id,omitempty"`
	StripeCustomerID     *string    `gorm:"type:text" json:"stripe_customer_id,omitempty"`
	CurrentPeriodStart   *time.Time `json:"current_period_start,omitempty"`
	CurrentPeriodEnd     *time.Time `json:"current_period_end,omitempty"`
	CancelAtPeriodEnd    bool       `gorm:"not null;default:false" json:"cancel_at_period_end"`
	CreatedAt            time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt            time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	Plan *Plan `gorm:"foreignKey:PlanID" json:"plan,omitempty"`
}

// TableName specifies the custom database table name in GORM.
func (StoreSubscription) TableName() string {
	return "subscriptions"
}

// UsageRecord represents usage record.
type UsageRecord struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	StoreID     uuid.UUID `gorm:"type:uuid;not null;index" json:"store_id"`
	FeatureKey  string    `gorm:"size:100;not null" json:"feature_key"`
	UsageCount  int       `gorm:"not null;default:0" json:"usage_count"`
	PeriodStart time.Time `gorm:"not null" json:"period_start"`
	PeriodEnd   time.Time `gorm:"not null" json:"period_end"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}
