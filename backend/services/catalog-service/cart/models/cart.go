// Package models defines cart items, session entities, and checkout request schemas.
package models

import "time"

// CartItem represents an individual product line item and quantity held in a user's shopping cart.
// It serves as the discrete purchase unit within cart manipulations, basket price calculation,
// and checkout event generation.
//
// BERP Documentation:
//
// Behavior:
//   - Encapsulates a product reference and the customer's intended purchase quantity.
//   - Manipulated via cart endpoints (add, update, delete item).
//   - When quantity is updated to zero or below, the item is removed from the cart.
//   - Forms the basis of stock availability checks and price lookups during checkout.
//
// Exceptions:
//   - ProductID must be a non-empty, valid UUID string referring to an existing catalog product.
//   - Quantity must be a positive integer (min: 1) for active cart items.
//   - Insufficient stock exception if Quantity exceeds available inventory during checkout.
//
// Returns / Relations:
//   - Belongs To: Cart (embedded within the Items slice of Cart).
//   - References: Product in "catalog.products" and stock in "catalog.inventory" via ProductID.
//   - JSON Representation: Serialized as a JSON object with "product_id" and "quantity".
//
// Parameters / Properties:
//   - ProductID (string): Unique identifier (UUID string) of the selected catalog product.
//   - Quantity (int): Positive number of units the customer intends to purchase.
//
// Example:
//
//	item := models.CartItem{
//	    ProductID: "d0000000-0000-0000-0000-000000000001",
//	    Quantity:  2,
//	}
type CartItem struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// Cart represents an active customer shopping cart session in the catalog service.
// It tracks accumulated line items, applied promotional vouchers, and session update timestamps.
// Persistence is backed by Redis (key: cart:user:{user_id}) with automatic expiration TTL.
//
// BERP Documentation:
//
// Behavior:
//   - Serves as the aggregate root for a customer's pre-purchase shopping session.
//   - Supports authenticated users and guest sessions identified by UserID.
//   - Applies promotional voucher codes (CouponCode) passed through to checkout.
//   - Maintained in Redis with a configurable TTL (e.g. 7 days), refreshing UpdatedAt on each modification.
//   - Emitted as a CheckoutEvent to the order service upon initiating checkout.
//
// Exceptions:
//   - Empty cart exception when attempting checkout with zero items.
//   - UserID must be a non-empty string.
//   - Invalid or expired voucher error if CouponCode is unrecognized during basket optimization.
//   - Item quantity limit errors if requested quantities exceed per-order maximums or warehouse inventory.
//
// Returns / Relations:
//   - Storage Layer: Redis key "cart:user:{user_id}" with TTL expiration.
//   - Has Many: CartItem ([]CartItem) representing the current contents of the basket.
//   - Outbound Event: Emitted to SQS as CheckoutEvent consumed by order-service.
//   - JSON Representation: Serialized as JSON with snake_case attributes.
//
// Parameters / Properties:
//   - UserID (string): Unique user identifier or guest session token owning this shopping cart.
//   - Items ([]CartItem): Collection of product line items currently placed in the cart.
//   - CouponCode (string): Optional promotional or discount coupon code applied to the order.
//   - UpdatedAt (time.Time): Timestamp of the last cart mutation (item added, modified, or removed).
//
// Example:
//
//	cart := models.Cart{
//	    UserID: "u12345-67890-abcdef",
//	    Items: []models.CartItem{
//	        {
//	            ProductID: "d0000000-0000-0000-0000-000000000001",
//	            Quantity:  2,
//	        },
//	        {
//	            ProductID: "d0000000-0000-0000-0000-000000000002",
//	            Quantity:  1,
//	        },
//	    },
//	    CouponCode: "SUMMER20",
//	    UpdatedAt:  time.Now().UTC(),
//	}
type Cart struct {
	UserID     string     `json:"user_id"`
	Items      []CartItem `json:"items"`
	CouponCode string     `json:"coupon_code,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
