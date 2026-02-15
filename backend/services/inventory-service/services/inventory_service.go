package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/yashrajoria/inventory-service/models"
	"github.com/yashrajoria/inventory-service/repository"
)

// InventoryService handles business logic for inventory operations
type InventoryService struct {
	repo repository.InventoryRepository
}

// NewInventoryService creates a new InventoryService
func NewInventoryService(repo repository.InventoryRepository) *InventoryService {
	return &InventoryService{repo: repo}
}

// GetStock returns the current inventory for a product
func (s *InventoryService) GetStock(ctx context.Context, productID string) (*models.Inventory, error) {
	inv, err := s.repo.Get(ctx, productID)
	if err != nil {
		return nil, err
	}
	return inv, nil
}

// SetStock initializes or updates inventory for a product (upsert).
// If the product already has an inventory record the available count and
// threshold are updated while preserving the current reserved count.
func (s *InventoryService) SetStock(ctx context.Context, req *models.SetStockRequest) (*models.Inventory, error) {
	existing, err := s.repo.Get(ctx, req.ProductID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("failed to check existing stock: %w", err)
	}

	now := time.Now().UTC()

	if existing != nil {
		// Upsert: add incoming available to current stock, update threshold
		newAvailable := existing.Available + req.Available
		updates := map[string]interface{}{
			"available":  newAvailable,
			"threshold":  req.Threshold,
			"updated_at": now.Format(time.RFC3339),
		}
		if err := s.repo.Update(ctx, req.ProductID, updates); err != nil {
			return nil, fmt.Errorf("failed to update stock: %w", err)
		}
		existing.Available = newAvailable
		existing.Threshold = req.Threshold
		existing.UpdatedAt = now
		log.Printf("[InventoryService] Stock updated (upsert) for product=%s available=%d (+%d) threshold=%d reserved=%d",
			req.ProductID, newAvailable, req.Available, req.Threshold, existing.Reserved)
		return existing, nil
	}

	// First time: create a new inventory record
	inv := &models.Inventory{
		ProductID: req.ProductID,
		Available: req.Available,
		Reserved:  0,
		Threshold: req.Threshold,
		UpdatedAt: now,
	}

	if err := s.repo.Set(ctx, inv); err != nil {
		return nil, fmt.Errorf("failed to set stock: %w", err)
	}

	log.Printf("[InventoryService] Stock created for product=%s available=%d threshold=%d",
		req.ProductID, req.Available, req.Threshold)

	return inv, nil
}

// UpdateStock partially updates inventory for a product
func (s *InventoryService) UpdateStock(ctx context.Context, productID string, req *models.UpdateStockRequest) (*models.Inventory, error) {
	// Verify the inventory exists
	_, err := s.repo.Get(ctx, productID)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	if req.Available != nil {
		updates["available"] = *req.Available
	}
	if req.Threshold != nil {
		updates["threshold"] = *req.Threshold
	}

	if err := s.repo.Update(ctx, productID, updates); err != nil {
		return nil, fmt.Errorf("failed to update stock: %w", err)
	}

	// Return updated inventory
	return s.repo.Get(ctx, productID)
}

// ReserveStock reserves inventory for order items
func (s *InventoryService) ReserveStock(ctx context.Context, req *models.ReserveRequest) ([]models.StockCheckResult, error) {
	results := make([]models.StockCheckResult, 0, len(req.Items))

	// First, check all items have sufficient stock
	for _, item := range req.Items {
		check, err := s.repo.CheckStock(ctx, item.ProductID, item.Quantity)
		if err != nil {
			return nil, fmt.Errorf("failed to check stock for product=%s: %w", item.ProductID, err)
		}
		if !check.IsSufficient {
			return nil, fmt.Errorf("insufficient stock for product=%s: available=%d requested=%d",
				item.ProductID, check.Available, item.Quantity)
		}
	}

	// Reserve each item
	for _, item := range req.Items {
		if err := s.repo.Reserve(ctx, item.ProductID, item.Quantity); err != nil {
			// Rollback previously reserved items
			for _, reserved := range results {
				_ = s.repo.Release(ctx, reserved.ProductID, reserved.Requested)
			}

			if errors.Is(err, repository.ErrInsufficientStock) {
				return nil, fmt.Errorf("insufficient stock for product=%s (race condition)", item.ProductID)
			}
			return nil, fmt.Errorf("failed to reserve stock for product=%s: %w", item.ProductID, err)
		}

		results = append(results, models.StockCheckResult{
			ProductID:    item.ProductID,
			Requested:    item.Quantity,
			IsSufficient: true,
		})
	}

	log.Printf("[InventoryService] Reserved stock for order=%s items=%d", req.OrderID, len(results))
	return results, nil
}

// ReleaseStock releases previously reserved stock (order cancelled/payment failed)
func (s *InventoryService) ReleaseStock(ctx context.Context, req *models.ReleaseRequest) error {
	for _, item := range req.Items {
		if err := s.repo.Release(ctx, item.ProductID, item.Quantity); err != nil {
			log.Printf("[InventoryService] Failed to release stock for product=%s qty=%d: %v",
				item.ProductID, item.Quantity, err)
			// Continue releasing other items even if one fails
			continue
		}
	}

	log.Printf("[InventoryService] Released stock for order=%s items=%d", req.OrderID, len(req.Items))
	return nil
}

// ConfirmStock permanently deducts reserved stock (payment succeeded)
func (s *InventoryService) ConfirmStock(ctx context.Context, req *models.ConfirmRequest) error {
	for _, item := range req.Items {
		if err := s.repo.Confirm(ctx, item.ProductID, item.Quantity); err != nil {
			log.Printf("[InventoryService] Failed to confirm stock for product=%s qty=%d: %v",
				item.ProductID, item.Quantity, err)
			continue
		}
	}

	log.Printf("[InventoryService] Confirmed stock for order=%s items=%d", req.OrderID, len(req.Items))
	return nil
}

// CheckStock checks stock availability for multiple items
func (s *InventoryService) CheckStock(ctx context.Context, items []models.ReserveItem) ([]models.StockCheckResult, error) {
	results := make([]models.StockCheckResult, 0, len(items))

	for _, item := range items {
		check, err := s.repo.CheckStock(ctx, item.ProductID, item.Quantity)
		if err != nil {
			return nil, fmt.Errorf("failed to check stock for product=%s: %w", item.ProductID, err)
		}
		results = append(results, *check)
	}

	return results, nil
}
