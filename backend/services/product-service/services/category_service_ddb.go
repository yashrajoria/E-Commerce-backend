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

	// Attach product counts to all categories
	for _, cat := range rootCategories {
		s.attachProductCounts(ctx, cat, categoryMap)
	}

	return rootCategories, nil
}

// attachProductCounts recursively attaches direct and total product counts to categories and their children.
func (s *CategoryServiceDDB) attachProductCounts(ctx context.Context, cat *models.Category, categoryMap map[uuid.UUID]*models.Category) {
	// Compute direct product count for this category
	directCount, err := s.productRepo.Count(ctx, map[string]interface{}{
		"category_ids": cat.ID.String(),
	})
	if err == nil {
		cat.DirectProductCount = int(directCount)
	}

	// Compute total product count (including descendants)
	totalCount := int(directCount) // start with direct products
	if len(cat.Children) > 0 {
		descendantCount := s.countDescendantProducts(ctx, cat, categoryMap)
		totalCount += descendantCount
	}
	cat.TotalProductCount = totalCount

	// Recursively process children
	for _, child := range cat.Children {
		s.attachProductCounts(ctx, child, categoryMap)
	}
}

// countDescendantProducts recursively counts all products in descendant categories.
func (s *CategoryServiceDDB) countDescendantProducts(ctx context.Context, cat *models.Category, categoryMap map[uuid.UUID]*models.Category) int {
	total := 0
	for _, child := range cat.Children {
		// Direct count for this child
		directCount, err := s.productRepo.Count(ctx, map[string]interface{}{
			"category_ids": child.ID.String(),
		})
		if err == nil {
			total += int(directCount)
		}
		// Recurse to grandchildren
		total += s.countDescendantProducts(ctx, child, categoryMap)
	}
	return total
}

// RecalculateProductCountsForCategory recalculates product counts for a specific category and updates storage.
// Called when products are created, deleted, or moved between categories.
func (s *CategoryServiceDDB) RecalculateProductCountsForCategory(ctx context.Context, categoryID uuid.UUID) error {
	_, err := s.repo.FindByID(ctx, categoryID)
	if err != nil {
		return err
	}

	// Calculate direct product count
	directCount, err := s.productRepo.Count(ctx, map[string]interface{}{
		"category_ids": categoryID.String(),
	})
	if err != nil {
		return fmt.Errorf("failed to count direct products: %w", err)
	}

	// Get all descendants and sum their direct counts
	descendants := s.getAllDescendantCategoryIDs(ctx, categoryID)
	totalCount := int(directCount)
	for _, descendantID := range descendants {
		count, err := s.productRepo.Count(ctx, map[string]interface{}{
			"category_ids": descendantID.String(),
		})
		if err != nil {
			return fmt.Errorf("failed to count products in descendant: %w", err)
		}
		totalCount += int(count)
	}

	// Update category with new counts
	updates := map[string]interface{}{
		"direct_product_count": int(directCount),
		"total_product_count":  totalCount,
		"updated_at":           time.Now().UTC().Format(time.RFC3339),
	}

	err = s.repo.Update(ctx, categoryID, updates)
	if err != nil {
		return fmt.Errorf("failed to update category counts: %w", err)
	}

	// Also update parent categories' total counts
	category, err := s.repo.FindByID(ctx, categoryID)
	if err == nil && len(category.ParentIDs) > 0 {
		for _, parentID := range category.ParentIDs {
			_ = s.RecalculateProductCountsForCategory(ctx, parentID) // Propagate up the tree
		}
	}

	return nil
}

// getAllDescendantCategoryIDs fetches all descendant category IDs for a given category.
func (s *CategoryServiceDDB) getAllDescendantCategoryIDs(ctx context.Context, categoryID uuid.UUID) []uuid.UUID {
	allCategories, err := s.repo.FindAll(ctx)
	if err != nil {
		return []uuid.UUID{}
	}

	descendants := []uuid.UUID{}
	s.collectDescendantIDs(categoryID, allCategories, &descendants)
	return descendants
}

// collectDescendantIDs recursively collects all descendant category IDs.
func (s *CategoryServiceDDB) collectDescendantIDs(categoryID uuid.UUID, allCategories []models.Category, descendants *[]uuid.UUID) {
	for i := range allCategories {
		cat := &allCategories[i]
		for _, parentID := range cat.ParentIDs {
			if parentID == categoryID {
				*descendants = append(*descendants, cat.ID)
				// Recurse to find grandchildren
				s.collectDescendantIDs(cat.ID, allCategories, descendants)
				break
			}
		}
	}
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
