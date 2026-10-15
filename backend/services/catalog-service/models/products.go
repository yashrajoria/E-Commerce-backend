package models

import (
	"time"

	"github.com/google/uuid"
)

// Product represents a retail merchandise item in the ShopSwift catalog.
// It serves as the primary domain and API model for product browsing, searching,
// pricing, inventory tracking, and multi-tenant store association.
// Persistence is backed by PostgreSQL in the catalog schema.
//
// BERP Documentation:
//
// Behavior:
//   - Serves as the central aggregate root for product information across the storefront and merchant portal.
//   - Encapsulates pricing, display attributes, branding, image galleries, and categorization.
//   - Links products to multi-vendor merchant stores via the optional StoreID attribute.
//   - Highlights special items via the IsFeatured flag for promotional displays and recommendations.
//   - Implements soft-deletion through DeletedAt to maintain order history integrity and prevent broken references.
//   - Synchronizes with Redis caches for high-throughput product retrieval and search indexing.
//
// Exceptions:
//   - Unique constraint violation on live SKU (idx_products_sku_live) when deleted_at IS NULL.
//   - Check constraint failure if Price is negative (price >= 0).
//   - Check constraint failure if Quantity is negative (quantity >= 0).
//   - Foreign key violation if associated CategoryIDs do not exist in catalog.categories.
//   - Invalid UUID format error if ID, StoreID, or any CategoryID cannot be parsed.
//
// Returns / Relations:
//   - Database Table: "catalog.products" (PostgreSQL schema catalog).
//   - Many-to-Many: Categories via join table "catalog.product_categories" (ON DELETE CASCADE on product_id).
//   - Has One: Inventory via "catalog.inventory" on foreign key product_id.
//   - JSON Representation: Serialized with "_id" for backward compatibility and standard snake_case fields.
//
// Parameters / Properties:
//   - ID (uuid.UUID): Unique primary key identifier for the product.
//   - Name (string): Human-readable product title or commercial name.
//   - Price (float64): Unit retail price in decimal format (e.g., 49.99).
//   - Quantity (int): Available on-hand quantity cache; authoritative real-time counts reside in catalog.inventory.
//   - Description (string): Detailed textual or markdown product summary and specifications.
//   - Images ([]string): Collection of hosted media/S3 URLs displaying product views.
//   - Brand (string): Commercial manufacturer, trademark, or vendor brand name.
//   - SKU (string): Stock Keeping Unit alphanumeric code; must be unique across all active (non-deleted) products.
//   - CategoryIDs ([]uuid.UUID): Slice of UUIDs linking this product to category records in catalog.product_categories.
//   - CategoryPath ([]string): Breadcrumb path names of associated categories (e.g., ["Electronics", "Audio", "Headphones"]).
//   - IsFeatured (bool): Promotional flag indicating featured placement on home and category banner feeds.
//   - StoreID (*uuid.UUID): Optional tenant/merchant store identifier for multi-vendor marketplace isolation.
//   - CreatedAt (time.Time): Timestamp when the product record was first created.
//   - UpdatedAt (time.Time): Timestamp when the product record was last modified.
//   - DeletedAt (*time.Time): Nullable timestamp populated upon soft-deletion; nil indicates an active product.
//
// Example:
//
//	storeID := uuid.MustParse("b0000000-0000-0000-0000-000000000001")
//	catID := uuid.MustParse("c0000000-0000-0000-0000-000000000001")
//	product := models.Product{
//	    ID:           uuid.New(),
//	    Name:         "Wireless Noise-Canceling Headphones",
//	    Price:        199.99,
//	    Quantity:     45,
//	    Description:  "High-fidelity wireless over-ear headphones with active noise cancellation.",
//	    Images:       []string{"https://s3.amazonaws.com/shopswift/headphones-1.jpg"},
//	    Brand:        "SoundPro",
//	    SKU:          "SP-WNC-001",
//	    CategoryIDs:  []uuid.UUID{catID},
//	    CategoryPath: []string{"Electronics", "Audio", "Headphones"},
//	    IsFeatured:   true,
//	    StoreID:      &storeID,
//	    CreatedAt:    time.Now().UTC(),
//	    UpdatedAt:    time.Now().UTC(),
//	    DeletedAt:    nil,
//	}
type Product struct {
	ID           uuid.UUID   `json:"_id"`
	Name         string      `json:"name"`
	Price        float64     `json:"price"`
	Quantity     int         `json:"quantity"`
	Description  string      `json:"description,omitempty"`
	Images       []string    `json:"images,omitempty"`
	Brand        string      `json:"brand,omitempty"`
	SKU          string      `json:"sku"`
	CategoryIDs  []uuid.UUID `json:"category_ids,omitempty"`
	CategoryPath []string    `json:"category_path,omitempty"`
	IsFeatured   bool        `json:"is_featured"`
	StoreID      *uuid.UUID  `json:"store_id,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
	DeletedAt    *time.Time  `json:"deleted_at,omitempty"`
}
