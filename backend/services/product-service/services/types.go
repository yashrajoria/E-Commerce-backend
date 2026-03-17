package services

import "github.com/google/uuid"

// ListProductsParams contains parameters for listing products with filters
type ListProductsParams struct {
	Page       int
	PerPage    int
	Sort       string
	IsFeatured *bool
	CategoryID []uuid.UUID
	MinPrice   *float64
	MaxPrice   *float64
	Brand      *string
	InStock    *bool
}

// ProductCreateRequest is the request payload for creating a product
type ProductCreateRequest struct {
	Name        string
	Description string
	Brand       string
	SKU         string
	Price       float64
	Quantity    int
	IsFeatured  bool
	Categories  []string
}

// ProductUpdateRequest is the strictly typed payload for updating products
type ProductUpdateRequest struct {
	Name         *string     `json:"name"`
	Description  *string     `json:"description"`
	Brand        *string     `json:"brand"`
	SKU          *string     `json:"sku"`
	Price        *float64    `json:"price"`
	Quantity     *int        `json:"quantity"`
	IsFeatured   *bool       `json:"is_featured"`
	CategoryIDs  []uuid.UUID `json:"category_ids"`
	CategoryPath []string    `json:"category_path"`
	Images       []string    `json:"images"`
}

// ProductInternalDTO is a lightweight product representation for internal service calls
type ProductInternalDTO struct {
	ID    uuid.UUID
	Name  string
	Price float64
	Stock int
}

// BulkDeleteRequest describes options to delete products in bulk
type BulkDeleteRequest struct {
	// IDs explicitly deleted (UUIDs)
	IDs []uuid.UUID
	// Delete products that belong to any of these category IDs
	CategoryIDs []uuid.UUID
	// If true, delete all products (ignores other fields)
	DeleteAll bool
}

// CategoryCreateRequest is the request payload for creating a category
type CategoryCreateRequest struct {
	Name        string   `json:"name" validate:"required"`
	ParentNames []string `json:"parent_names"`
	Image       string   `json:"image"`
	IsActive    bool     `json:"is_active"`
}
