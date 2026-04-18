package events

// ProductEvent represents events for product/category operations.
// Used for maintaining category product count projections and cross-service synchronization.
type ProductEvent struct {
	EventType  string                 `json:"event_type"`   // product_created, product_updated, product_deleted, etc.
	ProductID  string                 `json:"product_id"`
	CategoryIDs []string              `json:"category_ids,omitempty"`  // Categories this product belongs to
	OldCategoryIDs []string           `json:"old_category_ids,omitempty"` // For move/update operations
	Data       map[string]interface{} `json:"data"`
	Timestamp  int64                  `json:"timestamp"`
}

// NewProductCreatedEvent creates an event when a product is added to the catalog.
// Used to increment product counts in affected categories.
func NewProductCreatedEvent(productID string, categoryIDs []string) ProductEvent {
	return ProductEvent{
		EventType:   "product_created",
		ProductID:   productID,
		CategoryIDs: categoryIDs,
		Data: map[string]interface{}{
			"action": "increment_counts",
		},
	}
}

// NewProductDeletedEvent creates an event when a product is removed.
// Used to decrement product counts in affected categories.
func NewProductDeletedEvent(productID string, categoryIDs []string) ProductEvent {
	return ProductEvent{
		EventType:   "product_deleted",
		ProductID:   productID,
		CategoryIDs: categoryIDs,
		Data: map[string]interface{}{
			"action": "decrement_counts",
		},
	}
}

// NewProductCategoryChangedEvent creates an event when a product's category assignment changes.
// Used to update counts: decrement from old categories, increment to new ones.
func NewProductCategoryChangedEvent(productID string, oldCategoryIDs, newCategoryIDs []string) ProductEvent {
	return ProductEvent{
		EventType:      "product_category_changed",
		ProductID:      productID,
		OldCategoryIDs: oldCategoryIDs,
		CategoryIDs:    newCategoryIDs,
		Data: map[string]interface{}{
			"action": "update_counts",
		},
	}
}

// NewProductBulkImportedEvent creates an event when products are bulk imported.
// Used to recalculate all category counts.
func NewProductBulkImportedEvent(categoryIDs []string) ProductEvent {
	return ProductEvent{
		EventType:   "product_bulk_imported",
		CategoryIDs: categoryIDs,
		Data: map[string]interface{}{
			"action": "recalculate_counts",
		},
	}
}
