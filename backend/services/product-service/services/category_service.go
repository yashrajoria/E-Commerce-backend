package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yashrajoria/product-service/models"
	"github.com/yashrajoria/product-service/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type CategoryCreateRequest struct {
	Name        string   `json:"name" validate:"required"`
	ParentNames []string `json:"parent_names"`
	Image       string   `json:"image"`
	IsActive    bool     `json:"is_active"`
}

type CategoryService struct {
	repo *repository.CategoryRepository
}

func NewCategoryService(repo *repository.CategoryRepository) *CategoryService {
	return &CategoryService{repo: repo}
}

// CreateCategory handles the business logic for creating a single category.
func (s *CategoryService) CreateCategory(ctx context.Context, req CategoryCreateRequest) (*models.Category, error) {
	// Check for duplicates
	_, err := s.repo.FindByName(ctx, req.Name)
	if err == nil {
		return nil, fmt.Errorf("category with name '%s' already exists", req.Name)
	}
	if err != mongo.ErrNoDocuments {
		return nil, err // A real database error occurred
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

	_, err = s.repo.Create(ctx, newCategory)
	if err != nil {
		return nil, err
	}
	return newCategory, nil
}

// A helper to resolve parent categories and build the full ancestor list.
func (s *CategoryService) resolveAncestry(ctx context.Context, parentNames []string) (parentIDs, ancestorIDs []uuid.UUID, err error) {
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

func (s *CategoryService) GetCategoryTree(ctx context.Context) ([]*models.Category, error) {
	categories, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	categoryMap := make(map[uuid.UUID]*models.Category)
	// Use a pointer to the category in the slice to modify it directly.
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

func (s *CategoryService) UpdateCategory(ctx context.Context, id uuid.UUID, req CategoryCreateRequest) (int64, error) {
	parentIDs, ancestorIDs, err := s.resolveAncestry(ctx, req.ParentNames)
	if err != nil {
		return 0, err
	}

	updates := bson.M{
		"name":       req.Name,
		"image":      req.Image,
		"is_active":  req.IsActive,
		"parent_ids": parentIDs,
		"ancestors":  ancestorIDs,
		"slug":       strings.ToLower(strings.ReplaceAll(req.Name, " ", "-")),
	}

	result, err := s.repo.Update(ctx, id, updates)
	if err != nil {
		return 0, err
	}
	return result.ModifiedCount, nil
}

func (s *CategoryService) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	// Business rule: check for associated products before deleting.
	hasProducts, err := s.repo.HasProducts(ctx, id)
	if err != nil {
		return err
	}
	if hasProducts {
		return fmt.Errorf("cannot delete category with associated products")
	}

	result, err := s.repo.Delete(ctx, id)
	if err != nil {
		return err
	}
	if result.ModifiedCount == 0 {
		return mongo.ErrNoDocuments // Use a standard error for "not found"
	}
	return nil
}
