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

type StoreController struct {
	storeService services.StoreService
}

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
				"contact_email":     store.Settings.ContactEmail,
				"contact_phone":     store.Settings.ContactPhone,
				"address":           store.Settings.Address,
				"maintenance_mode":  store.Settings.MaintenanceMode,
				"shipping_config":   store.Settings.ShippingConfig,
			},
		},
	})
}

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

func (ctrl *StoreController) ListPlans(c *gin.Context) {
	plans, err := ctrl.storeService.ListPlans(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list plans"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"plans": plans})
}
