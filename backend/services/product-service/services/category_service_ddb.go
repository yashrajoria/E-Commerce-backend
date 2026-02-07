package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"product-service/models"
	"product-service/repository"

	"github.com/google/uuid"
)

// CategoryServiceDDB is a DynamoDB-backed category service
type CategoryServiceDDB struct {
	repo        repository.CategoryRepo
	productRepo repository.ProductRepo
}

func NewCategoryServiceDDB(repo repository.CategoryRepo, productRepo repository.ProductRepo) *CategoryServiceDDB {
	return &CategoryServiceDDB{repo: repo, productRepo: productRepo}
}

// CreateCategory handles the business logic for creating a single category.
func (s *CategoryServiceDDB) CreateCategory(ctx context.Context, req CategoryCreateRequest) (*models.Category, error) {
	// Check for duplicates
	_, err := s.repo.FindByName(ctx, req.Name)
	if err == nil {
		return nil, fmt.Errorf("category with name '%s' already exists", req.Name)
	}
	// Continue only if error is "not found", otherwise return error
	if !strings.Contains(err.Error(), "not found") {
		return nil, err
	}

	// Resolve parents and ancestors
	parentIDs, ancestorIDs, err := s.resolveAncestry(ctx, req.ParentNames)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	slug := strings.ToLower(strings.ReplaceAll(req.Name, " ", "-"))

	newCategory := &models.Category{
		ID:        uuid.New(),
		Name:      req.Name,
		ParentIDs: parentIDs,
		Ancestors: ancestorIDs,
		Image:     req.Image,
		Slug:      slug,
		IsActive:  req.IsActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	err = s.repo.Create(ctx, newCategory)
	if err != nil {
		return nil, err
	}
	return newCategory, nil
}

// resolveAncestry resolves parent categories and builds the full ancestor list.
func (s *CategoryServiceDDB) resolveAncestry(ctx context.Context, parentNames []string) (parentIDs, ancestorIDs []uuid.UUID, err error) {
	if len(parentNames) == 0 {
		return []uuid.UUID{}, []uuid.UUID{}, nil
	}

	parents, err := s.repo.FindByNames(ctx, parentNames)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find parent categories: %w", err)
	}
	if len(parents) != len(parentNames) {
		return nil, nil, fmt.Errorf("one or more parent categories not found")
	}

	ancestorSet := make(map[uuid.UUID]bool)
	for _, p := range parents {
		parentIDs = append(parentIDs, p.ID)
		ancestorSet[p.ID] = true
		for _, ancestor := range p.Ancestors {
			ancestorSet[ancestor] = true
		}
	}

	for id := range ancestorSet {
		ancestorIDs = append(ancestorIDs, id)
	}

	return parentIDs, ancestorIDs, nil
}

func (s *CategoryServiceDDB) GetCategoryTree(ctx context.Context) ([]*models.Category, error) {
	categories, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	categoryMap := make(map[uuid.UUID]*models.Category)
	for i := range categories {
		categoryMap[categories[i].ID] = &categories[i]
	}

	var rootCategories []*models.Category
	for i := range categories {
		cat := &categories[i]
		if len(cat.ParentIDs) == 0 {
			rootCategories = append(rootCategories, cat)
		} else {
			for _, parentID := range cat.ParentIDs {
				if parent, ok := categoryMap[parentID]; ok {
					parent.Children = append(parent.Children, cat)
				}
			}
		}
	}
	return rootCategories, nil
}

func (s *CategoryServiceDDB) UpdateCategory(ctx context.Context, id uuid.UUID, req CategoryCreateRequest) (int64, error) {
	parentIDs, ancestorIDs, err := s.resolveAncestry(ctx, req.ParentNames)
	if err != nil {
		return 0, err
	}

	updates := map[string]interface{}{
		"name":       req.Name,
		"image":      req.Image,
		"is_active":  req.IsActive,
		"parent_ids": parentIDs,
		"ancestors":  ancestorIDs,
		"slug":       strings.ToLower(strings.ReplaceAll(req.Name, " ", "-")),
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}

	err = s.repo.Update(ctx, id, updates)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func (s *CategoryServiceDDB) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	// Business rule: check for associated products before deleting.
	hasProducts, err := s.repo.HasProducts(ctx, id)
	if err != nil {
		return err
	}
	if hasProducts {
		return fmt.Errorf("cannot delete category with associated products")
	}

	err = s.repo.Delete(ctx, id)
	if err != nil {
		return err
	}
	return nil
}

// GetCategory returns a single category by ID
func (s *CategoryServiceDDB) GetCategory(ctx context.Context, id uuid.UUID) (*models.Category, error) {
	return s.repo.FindByID(ctx, id)
}

// FindByNames returns categories by their names
func (s *CategoryServiceDDB) FindByNames(ctx context.Context, names []string) ([]models.Category, error) {
	return s.repo.FindByNames(ctx, names)
}
