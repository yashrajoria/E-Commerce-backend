package services

import (
	"context"
	"fmt"
	"log"
	"time"

	awspkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"catalog-service/inventory/models"
	"catalog-service/inventory/repository"
)

// InventoryService handles business logic for inventory operations
type InventoryService struct {
	repo          repository.InventoryRepository
	metricsClient *awspkg.MetricsClient
}

// NewInventoryService creates a new InventoryService
func NewInventoryService(repo repository.InventoryRepository, metricsClient *awspkg.MetricsClient) *InventoryService {
	return &InventoryService{
		repo:          repo,
		metricsClient: metricsClient,
	}
}

// GetStock returns the current inventory for a product
func (s *InventoryService) GetStock(ctx context.Context, productID string) (*models.Inventory, error) {
	inv, err := s.repo.Get(ctx, productID)
	if err != nil {
		return nil, err
	}
	return inv, nil
}

// SetStock adds req.Available to the product's stock (creating the record if
// needed) and overwrites the threshold, atomically. The reserved count is kept.
func (s *InventoryService) SetStock(ctx context.Context, req *models.SetStockRequest) (*models.Inventory, error) {
	inv, err := s.repo.AddStock(ctx, req.ProductID, req.Available, req.Threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to set stock: %w", err)
	}
	log.Printf("[InventoryService] Stock set for product=%s available=%d (+%d) threshold=%d reserved=%d",
		req.ProductID, inv.Available, req.Available, inv.Threshold, inv.Reserved)
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

// ReserveStock reserves inventory for order items atomically and idempotently.
func (s *InventoryService) ReserveStock(ctx context.Context, req *models.ReserveRequest) ([]models.StockCheckResult, error) {
	// Call transactional repository method
	if err := s.repo.ReserveAll(ctx, req.OrderID, req.Items); err != nil {
		return nil, err
	}

	results := make([]models.StockCheckResult, 0, len(req.Items))
	for _, item := range req.Items {
		results = append(results, models.StockCheckResult{
			ProductID:    item.ProductID,
			Requested:    item.Quantity,
			IsSufficient: true,
		})

		// Emit metrics (async)
		if s.metricsClient != nil && s.metricsClient.IsEnabled() {
			go func(pID string, qty int) {
				mCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				dims := map[string]string{"ProductID": pID}
				_ = s.metricsClient.RecordCount(mCtx, awspkg.MetricInventoryReserved, dims)
				_ = s.metricsClient.RecordValue(mCtx, "InventoryReservedQuantity", float64(qty), dims)
			}(item.ProductID, item.Quantity)
		}
	}

	log.Printf("[InventoryService] Transactional reserve success for order=%s items=%d", req.OrderID, len(results))
	return results, nil
}

// ReleaseStock releases previously reserved stock atomically and idempotently.
func (s *InventoryService) ReleaseStock(ctx context.Context, req *models.ReleaseRequest) error {
	if err := s.repo.ReleaseAll(ctx, req.OrderID, req.Items); err != nil {
		return err
	}
	log.Printf("[InventoryService] Transactional release success for order=%s items=%d", req.OrderID, len(req.Items))
	return nil
}

// ConfirmStock permanently deducts reserved stock atomically and idempotently.
func (s *InventoryService) ConfirmStock(ctx context.Context, req *models.ConfirmRequest) error {
	if err := s.repo.ConfirmAll(ctx, req.OrderID, req.Items); err != nil {
		return err
	}
	log.Printf("[InventoryService] Transactional confirm success for order=%s items=%d", req.OrderID, len(req.Items))
	return nil
}

// RestockStock returns confirmed quantities to available stock when a paid
// order is cancelled/refunded. Must only be called for orders whose
// reservation was already confirmed (and thus removed).
func (s *InventoryService) RestockStock(ctx context.Context, req *models.RestockRequest) error {
	if err := s.repo.RestockAll(ctx, req.OrderID, req.Items); err != nil {
		return err
	}
	log.Printf("[InventoryService] Transactional restock success for order=%s items=%d", req.OrderID, len(req.Items))
	return nil
}

// CheckStock checks stock availability for multiple items with a single
// batched read instead of one query per line item.
func (s *InventoryService) CheckStock(ctx context.Context, items []models.ReserveItem) ([]models.StockCheckResult, error) {
	if len(items) == 0 {
		return []models.StockCheckResult{}, nil
	}

	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ProductID)
	}
	invMap, err := s.repo.BatchGet(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to batch check stock for %d products: %w", len(items), err)
	}

	results := make([]models.StockCheckResult, 0, len(items))
	for _, item := range items {
		inv, ok := invMap[item.ProductID]
		if !ok {
			results = append(results, models.StockCheckResult{
				ProductID:    item.ProductID,
				Available:    0,
				Reserved:     0,
				Requested:    item.Quantity,
				IsSufficient: false,
			})
			continue
		}
		results = append(results, models.StockCheckResult{
			ProductID:    item.ProductID,
			Available:    inv.Available,
			Reserved:     inv.Reserved,
			Requested:    item.Quantity,
			IsSufficient: inv.Available >= item.Quantity,
		})
	}

	return results, nil
}

// ListAllStock returns inventory page `page` (1-indexed) with `pageSize` items.
func (s *InventoryService) ListAllStock(ctx context.Context, page, pageSize int) ([]models.Inventory, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	items, err := s.repo.ListAll(ctx, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to list inventory: %w", err)
	}
	return items, nil
}
