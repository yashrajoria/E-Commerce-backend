package repository

import (
	"context"
	"errors"
	"fmt"
	"identity-service/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StoreRepository defines the interface contract for store repository.
type StoreRepository interface {
	CreateStore(ctx context.Context, store *models.Store, ownerID uuid.UUID) (*models.Store, error)
	FindByID(ctx context.Context, id uuid.UUID) (*models.Store, error)
	FindBySlug(ctx context.Context, slug string) (*models.Store, error)
	ListUserStores(ctx context.Context, userID uuid.UUID) ([]*models.Store, error)
	UpdateStore(ctx context.Context, store *models.Store) error
	DeleteStore(ctx context.Context, id uuid.UUID) error

	GetMembership(ctx context.Context, storeID, userID uuid.UUID) (*models.StoreMembership, error)
	ListMembers(ctx context.Context, storeID uuid.UUID) ([]models.StoreMembership, error)
	AddMember(ctx context.Context, membership *models.StoreMembership) error
	UpdateMemberRole(ctx context.Context, storeID, userID uuid.UUID, role models.StoreRole) error
	RemoveMember(ctx context.Context, storeID, userID uuid.UUID) error

	GetSettings(ctx context.Context, storeID uuid.UUID) (*models.StoreSettings, error)
	UpdateSettings(ctx context.Context, settings *models.StoreSettings) error

	GetSubscription(ctx context.Context, storeID uuid.UUID) (*models.StoreSubscription, error)
	ListPlans(ctx context.Context) ([]models.Plan, error)
}

type storeRepository struct {
	db *gorm.DB
}

// NewStoreRepository creates and initializes a new StoreRepository instance.
func NewStoreRepository(db *gorm.DB) StoreRepository {
	return &storeRepository{db: db}
}

// CreateStore persists a new e record in the database.
func (r *storeRepository) CreateStore(ctx context.Context, store *models.Store, ownerID uuid.UUID) (*models.Store, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Insert store
		if err := tx.Create(store).Error; err != nil {
			return err
		}

		// 2. Add owner membership
		membership := models.StoreMembership{
			UserID:   ownerID,
			StoreID:  store.ID,
			Role:     models.StoreRoleOwner,
			Accepted: true,
		}
		if err := tx.Create(&membership).Error; err != nil {
			return err
		}

		// 3. Initialize default settings
		settings := models.StoreSettings{
			StoreID:          store.ID,
			MaintenanceMode:  false,
			AnalyticsEnabled: true,
			ShippingConfig:   "{}",
			PaymentConfig:    "{}",
		}
		if err := tx.Create(&settings).Error; err != nil {
			return err
		}

		// 4. Attach Free plan subscription if free plan exists
		var freePlan models.Plan
		if err := tx.Where("slug = ?", "free").First(&freePlan).Error; err == nil {
			sub := models.StoreSubscription{
				StoreID: store.ID,
				PlanID:  freePlan.ID,
				Status:  "active",
			}
			if err := tx.Create(&sub).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return store, nil
}

// FindByID queries the database for  by id.
func (r *storeRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Store, error) {
	var store models.Store
	err := r.db.WithContext(ctx).
		Preload("Settings").
		Preload("Subscription.Plan").
		Where("id = ?", id).
		First(&store).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("store not found")
		}
		return nil, err
	}
	return &store, nil
}

// FindBySlug queries the database for  by slug.
func (r *storeRepository) FindBySlug(ctx context.Context, slug string) (*models.Store, error) {
	var store models.Store
	err := r.db.WithContext(ctx).
		Preload("Settings").
		Where("slug = ? AND is_active = true", slug).
		First(&store).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("store not found")
		}
		return nil, err
	}
	return &store, nil
}

// ListUserStores retrieves a collection of user stores from the database.
func (r *storeRepository) ListUserStores(ctx context.Context, userID uuid.UUID) ([]*models.Store, error) {
	var memberships []models.StoreMembership
	err := r.db.WithContext(ctx).
		Preload("Store.Settings").
		Preload("Store.Subscription.Plan").
		Where("user_id = ?", userID).
		Find(&memberships).Error
	if err != nil {
		return nil, err
	}

	stores := make([]*models.Store, 0, len(memberships))
	for _, m := range memberships {
		if m.Store != nil && m.Store.DeletedAt.Valid == false {
			stores = append(stores, m.Store)
		}
	}
	return stores, nil
}

// UpdateStore updates store in the database.
func (r *storeRepository) UpdateStore(ctx context.Context, store *models.Store) error {
	return r.db.WithContext(ctx).
		Model(store).
		Updates(map[string]interface{}{
			"name":        store.Name,
			"slug":        store.Slug,
			"description": store.Description,
			"logo_url":    store.LogoURL,
			"currency":    store.Currency,
			"timezone":    store.Timezone,
			"is_active":   store.IsActive,
		}).Error
}

// DeleteStore deletes store from the database.
func (r *storeRepository) DeleteStore(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&models.Store{}).Error
}

// GetMembership queries the database for membership.
func (r *storeRepository) GetMembership(ctx context.Context, storeID, userID uuid.UUID) (*models.StoreMembership, error) {
	var m models.StoreMembership
	err := r.db.WithContext(ctx).
		Where("store_id = ? AND user_id = ?", storeID, userID).
		First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// ListMembers retrieves a collection of members from the database.
func (r *storeRepository) ListMembers(ctx context.Context, storeID uuid.UUID) ([]models.StoreMembership, error) {
	var members []models.StoreMembership
	err := r.db.WithContext(ctx).
		Preload("User").
		Where("store_id = ?", storeID).
		Find(&members).Error
	return members, err
}

// AddMember performs add member on r.
func (r *storeRepository) AddMember(ctx context.Context, membership *models.StoreMembership) error {
	return r.db.WithContext(ctx).Create(membership).Error
}

// UpdateMemberRole updates member role in the database.
func (r *storeRepository) UpdateMemberRole(ctx context.Context, storeID, userID uuid.UUID, role models.StoreRole) error {
	return r.db.WithContext(ctx).
		Model(&models.StoreMembership{}).
		Where("store_id = ? AND user_id = ?", storeID, userID).
		Update("role", role).Error
}

// RemoveMember performs remove member on r.
func (r *storeRepository) RemoveMember(ctx context.Context, storeID, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("store_id = ? AND user_id = ?", storeID, userID).
		Delete(&models.StoreMembership{}).Error
}

// GetSettings queries the database for settings.
func (r *storeRepository) GetSettings(ctx context.Context, storeID uuid.UUID) (*models.StoreSettings, error) {
	var settings models.StoreSettings
	err := r.db.WithContext(ctx).Where("store_id = ?", storeID).First(&settings).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create default settings if not yet present
			settings = models.StoreSettings{
				StoreID:          storeID,
				MaintenanceMode:  false,
				AnalyticsEnabled: true,
				ShippingConfig:   "{}",
				PaymentConfig:    "{}",
			}
			_ = r.db.WithContext(ctx).Create(&settings).Error
			return &settings, nil
		}
		return nil, err
	}
	return &settings, nil
}

// UpdateSettings updates settings in the database.
func (r *storeRepository) UpdateSettings(ctx context.Context, settings *models.StoreSettings) error {
	return r.db.WithContext(ctx).Save(settings).Error
}

// GetSubscription queries the database for subscription.
func (r *storeRepository) GetSubscription(ctx context.Context, storeID uuid.UUID) (*models.StoreSubscription, error) {
	var sub models.StoreSubscription
	err := r.db.WithContext(ctx).
		Preload("Plan.Entitlements").
		Where("store_id = ?", storeID).
		First(&sub).Error
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

// ListPlans retrieves a collection of plans from the database.
func (r *storeRepository) ListPlans(ctx context.Context) ([]models.Plan, error) {
	var plans []models.Plan
	err := r.db.WithContext(ctx).
		Preload("Entitlements").
		Where("is_active = true").
		Find(&plans).Error
	return plans, err
}
