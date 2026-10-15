// Package controllers provides HTTP endpoints for store management, membership invites, and settings.
package controllers

import (
	"errors"
	"net/http"
	"strings"

	"identity-service/models"
	"identity-service/stores/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// StoreController represents store controller.
type StoreController struct {
	storeService services.StoreService
}

// NewStoreController creates and initializes a new StoreController instance.
func NewStoreController(ss services.StoreService) *StoreController {
	return &StoreController{storeService: ss}
}

func getUserContext(c *gin.Context) (uuid.UUID, bool) {
	uidVal, exists := c.Get("user_id")
	if !exists {
		// Fallback to header if present
		uidHeader := c.GetHeader("X-User-ID")
		if uidHeader != "" {
			uid, err := uuid.Parse(uidHeader)
			if err == nil {
				role := c.GetHeader("X-User-Role")
				return uid, role == "admin"
			}
		}
		return uuid.Nil, false
	}

	var uid uuid.UUID
	switch v := uidVal.(type) {
	case string:
		parsed, err := uuid.Parse(v)
		if err == nil {
			uid = parsed
		}
	case uuid.UUID:
		uid = v
	}

	roleVal, _ := c.Get("role")
	roleStr, _ := roleVal.(string)
	if roleStr == "" {
		roleStr = c.GetHeader("X-User-Role")
	}

	return uid, roleStr == "admin"
}

// CreateStore handles POST /stores requests to provision a new merchant store, assigning the caller as store owner.
func (ctrl *StoreController) CreateStore(c *gin.Context) {
	userID, _ := getUserContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req services.CreateStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	store, err := ctrl.storeService.CreateStore(c.Request.Context(), userID, req)
	if err != nil {
		zap.L().Error("Failed to create store", zap.Error(err))
		if errors.Is(err, services.ErrInvalidSlug) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create store"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"store": store})
}

// ListStores handles GET /stores requests, retrieving all stores where the authenticated caller is an active member.
func (ctrl *StoreController) ListStores(c *gin.Context) {
	userID, _ := getUserContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	stores, err := ctrl.storeService.ListUserStores(c.Request.Context(), userID)
	if err != nil {
		zap.L().Error("Failed to list user stores", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list stores"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"stores": stores})
}

// GetStore handles GET /stores/:id requests, returning detailed store metadata if the caller has sufficient membership access.
func (ctrl *StoreController) GetStore(c *gin.Context) {
	userID, isSystemAdmin := getUserContext(c)
	idStr := c.Param("id")
	storeID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}

	store, err := ctrl.storeService.GetStore(c.Request.Context(), storeID, userID, isSystemAdmin)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "store not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"store": store})
}

// GetStoreBySlug handles GET /stores/slug/:slug requests for public storefront lookup by unique store slug.
func (ctrl *StoreController) GetStoreBySlug(c *gin.Context) {
	slug := c.Param("slug")
	if strings.TrimSpace(slug) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug is required"})
		return
	}

	store, err := ctrl.storeService.GetStoreBySlug(c.Request.Context(), slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "store not found"})
		return
	}

	// For public lookup, only return public safe fields
	c.JSON(http.StatusOK, gin.H{
		"store": gin.H{
			"id":          store.ID,
			"name":        store.Name,
			"slug":        store.Slug,
			"description": store.Description,
			"logo_url":    store.LogoURL,
			"currency":    store.Currency,
			"timezone":    store.Timezone,
			"is_active":   store.IsActive,
			"settings": gin.H{
				"contact_email":    store.Settings.ContactEmail,
				"contact_phone":    store.Settings.ContactPhone,
				"address":          store.Settings.Address,
				"maintenance_mode": store.Settings.MaintenanceMode,
				"shipping_config":  store.Settings.ShippingConfig,
			},
		},
	})
}

// UpdateStore handles PUT /stores/:id requests to update store metadata including name, description, logo, and timezone.
func (ctrl *StoreController) UpdateStore(c *gin.Context) {
	userID, isSystemAdmin := getUserContext(c)
	idStr := c.Param("id")
	storeID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}

	var req services.UpdateStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updated, err := ctrl.storeService.UpdateStore(c.Request.Context(), storeID, userID, isSystemAdmin, req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update store"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"store": updated})
}

// DeleteStore handles DELETE /stores/:id requests, soft-deleting a store if the caller is the owner or a platform admin.
func (ctrl *StoreController) DeleteStore(c *gin.Context) {
	userID, isSystemAdmin := getUserContext(c)
	idStr := c.Param("id")
	storeID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}

	if err := ctrl.storeService.DeleteStore(c.Request.Context(), storeID, userID, isSystemAdmin); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete store"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "store deleted successfully"})
}

// ListMembers handles GET /stores/:id/members requests, listing team members and their roles within the store.
func (ctrl *StoreController) ListMembers(c *gin.Context) {
	userID, isSystemAdmin := getUserContext(c)
	idStr := c.Param("id")
	storeID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}

	members, err := ctrl.storeService.ListMembers(c.Request.Context(), storeID, userID, isSystemAdmin)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list members"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"members": members})
}

// InviteMember handles POST /stores/:id/members requests to invite a user to the store team with an assigned role.
func (ctrl *StoreController) InviteMember(c *gin.Context) {
	userID, isSystemAdmin := getUserContext(c)
	idStr := c.Param("id")
	storeID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}

	var body struct {
		Email string           `json:"email" binding:"required"`
		Role  models.StoreRole `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	membership, err := ctrl.storeService.InviteMember(c.Request.Context(), storeID, userID, isSystemAdmin, body.Email, body.Role)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"membership": membership})
}

// UpdateMemberRole handles PUT /stores/:id/members/:userId requests to modify a member's role (owner, admin, manager, viewer).
func (ctrl *StoreController) UpdateMemberRole(c *gin.Context) {
	userID, isSystemAdmin := getUserContext(c)
	storeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}
	targetUserID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid member user id"})
		return
	}

	var body struct {
		Role models.StoreRole `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.storeService.UpdateMemberRole(c.Request.Context(), storeID, userID, isSystemAdmin, targetUserID, body.Role); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update member role"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "member role updated"})
}

// RemoveMember handles DELETE /stores/:id/members/:userId requests to revoke a user's membership from the store.
func (ctrl *StoreController) RemoveMember(c *gin.Context) {
	userID, isSystemAdmin := getUserContext(c)
	storeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}
	targetUserID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid member user id"})
		return
	}

	if err := ctrl.storeService.RemoveMember(c.Request.Context(), storeID, userID, isSystemAdmin, targetUserID); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove member"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "member removed"})
}

// GetSettings handles GET /stores/:id/settings requests, retrieving operational, shipping, and payment configuration.
func (ctrl *StoreController) GetSettings(c *gin.Context) {
	userID, isSystemAdmin := getUserContext(c)
	storeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}

	settings, err := ctrl.storeService.GetSettings(c.Request.Context(), storeID, userID, isSystemAdmin)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "settings not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"settings": settings})
}

// UpdateSettings handles PUT /stores/:id/settings requests to update operational settings, maintenance mode, and carrier configs.
func (ctrl *StoreController) UpdateSettings(c *gin.Context) {
	userID, isSystemAdmin := getUserContext(c)
	storeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}

	var settings models.StoreSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	settings.StoreID = storeID

	updated, err := ctrl.storeService.UpdateSettings(c.Request.Context(), storeID, userID, isSystemAdmin, &settings)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update settings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"settings": updated})
}

// GetSubscription handles GET /stores/:id/subscription requests, retrieving active subscription plan and billing period details.
func (ctrl *StoreController) GetSubscription(c *gin.Context) {
	userID, isSystemAdmin := getUserContext(c)
	storeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}

	sub, err := ctrl.storeService.GetSubscription(c.Request.Context(), storeID, userID, isSystemAdmin)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"subscription": sub})
}

// ListPlans handles GET /stores/plans requests, returning available SaaS subscription tiers and feature entitlements.
func (ctrl *StoreController) ListPlans(c *gin.Context) {
	plans, err := ctrl.storeService.ListPlans(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list plans"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"plans": plans})
}

// GetMyMembership handles GET /stores/:id/my-membership requests, returning the authenticated user's role and permissions.
func (ctrl *StoreController) GetMyMembership(c *gin.Context) {
	userID, _ := getUserContext(c)
	storeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}
	role, err := ctrl.storeService.MembershipRole(c.Request.Context(), storeID, userID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a store member"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"store_id": storeID, "user_id": userID, "role": role})
}
