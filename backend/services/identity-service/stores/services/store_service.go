package services

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"identity-service/models"
	"identity-service/repository"

	"github.com/google/uuid"
)

var (
	ErrStoreNotFound     = errors.New("store not found")
	ErrForbidden         = errors.New("forbidden: insufficient permissions")
	ErrInvalidSlug       = errors.New("invalid slug: must be alphanumeric and hyphens only")
	ErrStoreLimitReached = errors.New("store limit reached for your current plan")
)

type CreateStoreRequest struct {
	Name        string  `json:"name" binding:"required"`
	Slug        string  `json:"slug"`
	Description *string `json:"description"`
	LogoURL     *string `json:"logo_url"`
	Currency    string  `json:"currency"`
	Timezone    string  `json:"timezone"`
}

type UpdateStoreRequest struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description *string `json:"description"`
	LogoURL     *string `json:"logo_url"`
	Currency    string  `json:"currency"`
	Timezone    string  `json:"timezone"`
	IsActive    *bool   `json:"is_active"`
}

type StoreService interface {
	CreateStore(ctx context.Context, userID uuid.UUID, req CreateStoreRequest) (*models.Store, error)
	GetStore(ctx context.Context, storeID, userID uuid.UUID, isSystemAdmin bool) (*models.Store, error)
	GetStoreBySlug(ctx context.Context, slug string) (*models.Store, error)
	ListUserStores(ctx context.Context, userID uuid.UUID) ([]*models.Store, error)
	UpdateStore(ctx context.Context, storeID, userID uuid.UUID, isSystemAdmin bool, req UpdateStoreRequest) (*models.Store, error)
	DeleteStore(ctx context.Context, storeID, userID uuid.UUID, isSystemAdmin bool) error

	ListMembers(ctx context.Context, storeID, userID uuid.UUID, isSystemAdmin bool) ([]models.StoreMembership, error)
	InviteMember(ctx context.Context, storeID, currentUserID uuid.UUID, isSystemAdmin bool, email string, role models.StoreRole) (*models.StoreMembership, error)
	UpdateMemberRole(ctx context.Context, storeID, currentUserID uuid.UUID, isSystemAdmin bool, targetUserID uuid.UUID, role models.StoreRole) error
	RemoveMember(ctx context.Context, storeID, currentUserID uuid.UUID, isSystemAdmin bool, targetUserID uuid.UUID) error

	GetSettings(ctx context.Context, storeID, userID uuid.UUID, isSystemAdmin bool) (*models.StoreSettings, error)
	UpdateSettings(ctx context.Context, storeID, userID uuid.UUID, isSystemAdmin bool, settings *models.StoreSettings) (*models.StoreSettings, error)

	GetSubscription(ctx context.Context, storeID, userID uuid.UUID, isSystemAdmin bool) (*models.StoreSubscription, error)
	ListPlans(ctx context.Context) ([]models.Plan, error)
}

type storeService struct {
	storeRepo repository.StoreRepository
	userRepo  *repository.UserRepository
}

func NewStoreService(sr repository.StoreRepository, ur *repository.UserRepository) StoreService {
	return &storeService{storeRepo: sr, userRepo: ur}
}

var slugRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func sanitizeSlug(input string) string {
	s := strings.ToLower(strings.TrimSpace(input))
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func (s *storeService) checkPermission(ctx context.Context, storeID, userID uuid.UUID, allowedRoles ...models.StoreRole) (bool, error) {
	m, err := s.storeRepo.GetMembership(ctx, storeID, userID)
	if err != nil {
		return false, err
	}
	if m == nil {
		return false, ErrForbidden
	}
	for _, role := range allowedRoles {
		if m.Role == role {
			return true, nil
		}
	}
	return false, ErrForbidden
}

func (s *storeService) CreateStore(ctx context.Context, userID uuid.UUID, req CreateStoreRequest) (*models.Store, error) {
	slug := sanitizeSlug(req.Slug)
	if slug == "" {
		slug = sanitizeSlug(req.Name)
	}
	if !slugRegex.MatchString(slug) {
		return nil, ErrInvalidSlug
	}

	currency := req.Currency
	if currency == "" {
		currency = "GBP"
	}
	timezone := req.Timezone
	if timezone == "" {
		timezone = "Europe/London"
	}

	store := &models.Store{
		Name:        strings.TrimSpace(req.Name),
		Slug:        slug,
		Description: req.Description,
		LogoURL:     req.LogoURL,
		Currency:    currency,
		Timezone:    timezone,
		IsActive:    true,
	}

	return s.storeRepo.CreateStore(ctx, store, userID)
}

func (s *storeService) GetStore(ctx context.Context, storeID, userID uuid.UUID, isSystemAdmin bool) (*models.Store, error) {
	if !isSystemAdmin {
		if _, err := s.checkPermission(ctx, storeID, userID, models.StoreRoleOwner, models.StoreRoleAdmin, models.StoreRoleManager, models.StoreRoleViewer); err != nil {
			return nil, err
		}
	}
	return s.storeRepo.FindByID(ctx, storeID)
}

func (s *storeService) GetStoreBySlug(ctx context.Context, slug string) (*models.Store, error) {
	cleanSlug := strings.ToLower(strings.TrimSpace(slug))
	return s.storeRepo.FindBySlug(ctx, cleanSlug)
}

func (s *storeService) ListUserStores(ctx context.Context, userID uuid.UUID) ([]*models.Store, error) {
	return s.storeRepo.ListUserStores(ctx, userID)
}

func (s *storeService) UpdateStore(ctx context.Context, storeID, userID uuid.UUID, isSystemAdmin bool, req UpdateStoreRequest) (*models.Store, error) {
	if !isSystemAdmin {
		if _, err := s.checkPermission(ctx, storeID, userID, models.StoreRoleOwner, models.StoreRoleAdmin); err != nil {
			return nil, err
		}
	}

	store, err := s.storeRepo.FindByID(ctx, storeID)
	if err != nil {
		return nil, err
	}

	if req.Name != "" {
		store.Name = strings.TrimSpace(req.Name)
	}
	if req.Slug != "" {
		newSlug := sanitizeSlug(req.Slug)
		if !slugRegex.MatchString(newSlug) {
			return nil, ErrInvalidSlug
		}
		store.Slug = newSlug
	}
	if req.Description != nil {
		store.Description = req.Description
	}
	if req.LogoURL != nil {
		store.LogoURL = req.LogoURL
	}
	if req.Currency != "" {
		store.Currency = req.Currency
	}
	if req.Timezone != "" {
		store.Timezone = req.Timezone
	}
	if req.IsActive != nil {
		store.IsActive = *req.IsActive
	}

	if err := s.storeRepo.UpdateStore(ctx, store); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *storeService) DeleteStore(ctx context.Context, storeID, userID uuid.UUID, isSystemAdmin bool) error {
	if !isSystemAdmin {
		if _, err := s.checkPermission(ctx, storeID, userID, models.StoreRoleOwner); err != nil {
			return err
		}
	}
	return s.storeRepo.DeleteStore(ctx, storeID)
}

func (s *storeService) ListMembers(ctx context.Context, storeID, userID uuid.UUID, isSystemAdmin bool) ([]models.StoreMembership, error) {
	if !isSystemAdmin {
		if _, err := s.checkPermission(ctx, storeID, userID, models.StoreRoleOwner, models.StoreRoleAdmin, models.StoreRoleManager, models.StoreRoleViewer); err != nil {
			return nil, err
		}
	}
	return s.storeRepo.ListMembers(ctx, storeID)
}

func (s *storeService) InviteMember(ctx context.Context, storeID, currentUserID uuid.UUID, isSystemAdmin bool, email string, role models.StoreRole) (*models.StoreMembership, error) {
	if !isSystemAdmin {
		if _, err := s.checkPermission(ctx, storeID, currentUserID, models.StoreRoleOwner, models.StoreRoleAdmin); err != nil {
			return nil, err
		}
	}

	targetUser, err := s.userRepo.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return nil, fmt.Errorf("user with email %s not found: %w", email, err)
	}

	existing, _ := s.storeRepo.GetMembership(ctx, storeID, targetUser.ID)
	if existing != nil {
		return nil, errors.New("user is already a member of this store")
	}

	membership := &models.StoreMembership{
		UserID:    targetUser.ID,
		StoreID:   storeID,
		Role:      role,
		InvitedBy: &currentUserID,
		Accepted:  true,
	}

	if err := s.storeRepo.AddMember(ctx, membership); err != nil {
		return nil, err
	}

	return membership, nil
}

func (s *storeService) UpdateMemberRole(ctx context.Context, storeID, currentUserID uuid.UUID, isSystemAdmin bool, targetUserID uuid.UUID, role models.StoreRole) error {
	if !isSystemAdmin {
		if _, err := s.checkPermission(ctx, storeID, currentUserID, models.StoreRoleOwner); err != nil {
			return err
		}
	}
	return s.storeRepo.UpdateMemberRole(ctx, storeID, targetUserID, role)
}

func (s *storeService) RemoveMember(ctx context.Context, storeID, currentUserID uuid.UUID, isSystemAdmin bool, targetUserID uuid.UUID) error {
	if !isSystemAdmin {
		if _, err := s.checkPermission(ctx, storeID, currentUserID, models.StoreRoleOwner); err != nil {
			return err
		}
	}
	return s.storeRepo.RemoveMember(ctx, storeID, targetUserID)
}

func (s *storeService) GetSettings(ctx context.Context, storeID, userID uuid.UUID, isSystemAdmin bool) (*models.StoreSettings, error) {
	if !isSystemAdmin {
		if _, err := s.checkPermission(ctx, storeID, userID, models.StoreRoleOwner, models.StoreRoleAdmin, models.StoreRoleManager, models.StoreRoleViewer); err != nil {
			return nil, err
		}
	}
	return s.storeRepo.GetSettings(ctx, storeID)
}

func (s *storeService) UpdateSettings(ctx context.Context, storeID, userID uuid.UUID, isSystemAdmin bool, settings *models.StoreSettings) (*models.StoreSettings, error) {
	if !isSystemAdmin {
		if _, err := s.checkPermission(ctx, storeID, userID, models.StoreRoleOwner, models.StoreRoleAdmin); err != nil {
			return nil, err
		}
	}

	current, err := s.storeRepo.GetSettings(ctx, storeID)
	if err != nil {
		return nil, err
	}

	if settings.ContactEmail != nil {
		current.ContactEmail = settings.ContactEmail
	}
	if settings.ContactPhone != nil {
		current.ContactPhone = settings.ContactPhone
	}
	if settings.Address != nil {
		current.Address = settings.Address
	}
	if settings.SMTPHost != nil {
		current.SMTPHost = settings.SMTPHost
	}
	if settings.SMTPPort != nil {
		current.SMTPPort = settings.SMTPPort
	}
	current.MaintenanceMode = settings.MaintenanceMode
	current.AnalyticsEnabled = settings.AnalyticsEnabled
	if len(settings.ShippingConfig) > 0 {
		current.ShippingConfig = settings.ShippingConfig
	} else if len(current.ShippingConfig) == 0 {
		current.ShippingConfig = "{}"
	}
	if len(settings.PaymentConfig) > 0 {
		current.PaymentConfig = settings.PaymentConfig
	} else if len(current.PaymentConfig) == 0 {
		current.PaymentConfig = "{}"
	}

	if err := s.storeRepo.UpdateSettings(ctx, current); err != nil {
		return nil, err
	}
	return current, nil
}

func (s *storeService) GetSubscription(ctx context.Context, storeID, userID uuid.UUID, isSystemAdmin bool) (*models.StoreSubscription, error) {
	if !isSystemAdmin {
		if _, err := s.checkPermission(ctx, storeID, userID, models.StoreRoleOwner, models.StoreRoleAdmin, models.StoreRoleManager, models.StoreRoleViewer); err != nil {
			return nil, err
		}
	}
	return s.storeRepo.GetSubscription(ctx, storeID)
}

func (s *storeService) ListPlans(ctx context.Context) ([]models.Plan, error) {
	return s.storeRepo.ListPlans(ctx)
}
