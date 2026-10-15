package models

import (
	"time"

	"github.com/google/uuid"
)

// Category represents a hierarchical navigation and classification entity in the catalog taxonomy.
// It organizes products into multi-level categories, tracks breadcrumbs and parent-child
// relationships, and calculates direct and aggregate product counts.
// Persistence is backed by PostgreSQL in the catalog schema.
//
// BERP Documentation:
//
// Behavior:
//   - Serves as the primary taxonomy classifier for organizing merchandise in the catalog.
//   - Supports directed acyclic graph (DAG) or tree hierarchies via ParentIDs, Ancestors, and Path.
//   - Maintains direct product counts (DirectProductCount) and recursive subtree totals (TotalProductCount).
//   - Provides URL-friendly identifiers via Slug for SEO and routing.
//   - Enables or disables public storefront visibility via the IsActive flag.
//   - In-memory hierarchical nesting is represented by the transient Children slice for tree rendering.
//   - Implements soft-deletion through DeletedAt to prevent dangling category associations.
//
// Exceptions:
//   - Level check constraint violation if Level is negative (level >= 0).
//   - Circular ancestry error during tree construction if a category lists itself or a descendant as parent.
//   - Invalid UUID format error if ID, StoreID, or any entry in ParentIDs or Ancestors is malformed.
//   - Name and slug collisions are permitted across branches, but must resolve unambiguously in route paths.
//
// Returns / Relations:
//   - Database Table: "catalog.categories" (PostgreSQL schema catalog).
//   - Many-to-Many: Products via join table "catalog.product_categories" (category_id, product_id).
//   - Self-Referencing Hierarchy: Parent/child relationships tracked via jsonb columns parent_ids, ancestors, path.
//   - In-Memory Tree: Nested Children ([]*Category) populated during hierarchical tree assembly.
//   - JSON Representation: Serialized with "_id" for backward compatibility and camelCase for count fields.
//
// Parameters / Properties:
//   - ID (uuid.UUID): Unique primary key identifier for the category.
//   - Name (string): Human-readable category display title (e.g., "Smartphones").
//   - ParentIDs ([]uuid.UUID): Slice of direct parent category UUIDs; empty for top-level root categories.
//   - Image (string): S3 URL or asset path for category iconography and promotional headers.
//   - Ancestors ([]uuid.UUID): Ordered array of ancestor UUIDs ascending to the root category.
//   - Slug (string): URL-friendly kebab-case slug for search-engine friendly URLs (e.g., "smartphones").
//   - Path ([]string): Hierarchical breadcrumb names representing category taxonomy path.
//   - Level (int): Zero-based depth level in the taxonomy tree (0 for root).
//   - IsActive (bool): Visibility flag determining whether the category is published and browsable.
//   - DirectProductCount (int): Number of active products directly assigned to this category node.
//   - TotalProductCount (int): Total number of active products in this category and all its descendants.
//   - StoreID (*uuid.UUID): Optional tenant/merchant store identifier for vendor-specific catalog categories.
//   - CreatedAt (time.Time): Timestamp when the category was created.
//   - UpdatedAt (time.Time): Timestamp when the category was last updated.
//   - DeletedAt (*time.Time): Nullable timestamp set upon soft-deletion; nil indicates an active category.
//   - Children ([]*Category): Transient slice of child categories populated dynamically for tree responses.
//
// Example:
//
//	parentID := uuid.MustParse("c0000000-0000-0000-0000-000000000001")
//	category := models.Category{
//	    ID:                 uuid.New(),
//	    Name:               "Wireless Earbuds",
//	    ParentIDs:          []uuid.UUID{parentID},
//	    Image:              "https://s3.amazonaws.com/shopswift/categories/earbuds.png",
//	    Ancestors:          []uuid.UUID{parentID},
//	    Slug:               "wireless-earbuds",
//	    Path:               []string{"Electronics", "Audio", "Wireless Earbuds"},
//	    Level:              2,
//	    IsActive:           true,
//	    DirectProductCount: 24,
//	    TotalProductCount:  24,
//	    StoreID:            nil,
//	    CreatedAt:          time.Now().UTC(),
//	    UpdatedAt:          time.Now().UTC(),
//	    DeletedAt:          nil,
//	    Children:           []*models.Category{},
//	}
type Category struct {
	ID                 uuid.UUID   `json:"_id"`
	Name               string      `json:"name"`
	ParentIDs          []uuid.UUID `json:"parent_ids,omitempty"`
	Image              string      `json:"image,omitempty"`
	Ancestors          []uuid.UUID `json:"ancestors,omitempty"`
	Slug               string      `json:"slug"`
	Path               []string    `json:"path,omitempty"`
	Level              int         `json:"level,omitempty"`
	IsActive           bool        `json:"is_active"`
	DirectProductCount int         `json:"directProductCount"` // Products directly in this category
	TotalProductCount  int         `json:"totalProductCount"`  // Including descendants
	StoreID            *uuid.UUID  `json:"store_id,omitempty"`
	CreatedAt          time.Time   `json:"created_at"`
	UpdatedAt          time.Time   `json:"updated_at"`
	DeletedAt          *time.Time  `json:"deleted_at,omitempty"`

	Children []*Category `json:"children,omitempty"` // transient field for frontend
}
