package repository

import (
	"context"

	"product-service/models"

	"github.com/google/uuid"
)

// ProductRepo defines the operations used by product-service.
// This interface uses plain Go types (no mongo-driver types) to make swapping adapters easier.
type ProductRepo interface {
	FindByID(ctx context.Context, id uuid.UUID) (*models.Product, error)
	Find(ctx context.Context, filter map[string]interface{}, limit, skip int) ([]*models.Product, error)
	Count(ctx context.Context, filter map[string]interface{}) (int64, error)
	Create(ctx context.Context, product *models.Product) error
	CreateMany(ctx context.Context, products []models.Product) error
	Update(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error
	Delete(ctx context.Context, id uuid.UUID) error
	// DeleteMany deletes multiple products by their UUIDs using batch operations
	DeleteMany(ctx context.Context, ids []uuid.UUID) error
	FindBySKUs(ctx context.Context, skus []string) ([]models.Product, error)
	EnsureIndexes(ctx context.Context) error
}

// CategoryRepo defines the operations used for category management.
type CategoryRepo interface {
	FindByID(ctx context.Context, id uuid.UUID) (*models.Category, error)
	FindByName(ctx context.Context, name string) (*models.Category, error)
	FindByNames(ctx context.Context, names []string) ([]models.Category, error)
	FindAll(ctx context.Context) ([]models.Category, error)
	Create(ctx context.Context, category *models.Category) error
	Update(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error
	Delete(ctx context.Context, id uuid.UUID) error
	HasProducts(ctx context.Context, categoryID uuid.UUID) (bool, error)
}
