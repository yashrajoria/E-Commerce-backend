package credit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/yashrajoria/common/internalauth"
	"order-service/middleware"
)

// Controller represents controller.
type Controller struct {
	service     *Service
	identityURL string
	client      *http.Client
}

// NewController creates and initializes a new Controller instance.
func NewController(s *Service) *Controller {
	url := os.Getenv("USER_SERVICE_URL")
	if url == "" {
		url = os.Getenv("IDENTITY_SERVICE_URL")
	}
	if url == "" {
		url = "http://identity-service:8081"
	}
	return &Controller{
		service:     s,
		identityURL: url,
		client:      &http.Client{Timeout: 3 * time.Second},
	}
}

func storeAndUser(c *gin.Context) (uuid.UUID, uuid.UUID, error) {
	storeID, err := uuid.Parse(c.GetHeader("X-Store-ID"))
	if err != nil {
		return uuid.Nil, uuid.Nil, errors.New("valid X-Store-ID header is required")
	}
	raw, err := middleware.GetUserID(c)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	userID, err := uuid.Parse(raw)
	return storeID, userID, err
}

func (cc *Controller) getMembershipRole(ctx context.Context, storeID, userID uuid.UUID) (string, error) {
	url := fmt.Sprintf("%s/stores/%s/membership/me", strings.TrimRight(cc.identityURL, "/"), storeID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-User-ID", userID.String())
	internalauth.Apply(req)

	resp, err := cc.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("not a member")
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(body.Role)), nil
}

func (cc *Controller) isAllowedMutator(c *gin.Context, storeID, actorID uuid.UUID) bool {
	globalRole := strings.ToLower(strings.TrimSpace(c.GetHeader("X-User-Role")))
	if globalRole == "admin" {
		return true
	}
	role, err := cc.getMembershipRole(c.Request.Context(), storeID, actorID)
	if err != nil {
		return false
	}
	return role == "owner" || role == "admin"
}

func (cc *Controller) isAllowedViewer(c *gin.Context, storeID, actorID uuid.UUID) bool {
	globalRole := strings.ToLower(strings.TrimSpace(c.GetHeader("X-User-Role")))
	if globalRole == "admin" {
		return true
	}
	role, err := cc.getMembershipRole(c.Request.Context(), storeID, actorID)
	if err != nil {
		return false
	}
	return role == "owner" || role == "admin" || role == "manager"
}

// Balance handles GET /store-credit/balance, retrieving available and held store credit amounts for a user in a store.
func (cc *Controller) Balance(c *gin.Context) {
	s, u, err := storeAndUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	target := u
	if raw := c.Query("user_id"); raw != "" {
		target, err = uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
			return
		}
		if target != u && !cc.isAllowedViewer(c, s, u) {
			c.JSON(http.StatusForbidden, gin.H{"error": "store manager, admin, or owner access required to view customer credit"})
			return
		}
	}

	avail, held, cur, err := cc.service.Balance(c.Request.Context(), s, target)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load store credit"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"available_amount": avail,
		"held_amount":      held,
		"total_amount":     avail + held,
		"currency":         cur,
	})
}

// Ledger handles GET /store-credit/ledger, returning a paginated history of credit ledger transactions.
func (cc *Controller) Ledger(c *gin.Context) {
	s, u, err := storeAndUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	target := u
	if raw := c.Query("user_id"); raw != "" {
		target, err = uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
			return
		}
		if target != u && !cc.isAllowedViewer(c, s, u) {
			c.JSON(http.StatusForbidden, gin.H{"error": "store manager, admin, or owner access required to view customer ledger"})
			return
		}
	}

	rows, err := cc.service.Ledger(c.Request.Context(), s, target, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load ledger"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": rows})
}

// Grant handles POST /store-credit/grants (admin/owner only), adding store credit to a customer's account.
func (cc *Controller) Grant(c *gin.Context) {
	s, actor, err := storeAndUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !cc.isAllowedMutator(c, s, actor) {
		c.JSON(http.StatusForbidden, gin.H{"error": "store owner or admin access required to issue credit"})
		return
	}

	idem := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idem == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Idempotency-Key header is required"})
		return
	}

	var req struct {
		UserID   string `json:"user_id" binding:"required"`
		Amount   int64  `json:"amount" binding:"required,gt=0"`
		Currency string `json:"currency"`
		Reason   string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: user_id, amount (>0), and reason are required"})
		return
	}

	target, err := uuid.Parse(req.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	if err := cc.service.Grant(c.Request.Context(), s, target, req.Amount, req.Currency, req.Reason, idem, &actor, nil); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusCreated)
}

// Adjust handles POST /store-credit/adjustments (admin/owner only), applying positive or negative balance corrections.
func (cc *Controller) Adjust(c *gin.Context) {
	s, actor, err := storeAndUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !cc.isAllowedMutator(c, s, actor) {
		c.JSON(http.StatusForbidden, gin.H{"error": "store owner or admin access required to adjust credit"})
		return
	}

	idem := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idem == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Idempotency-Key header is required"})
		return
	}

	var req struct {
		UserID   string `json:"user_id" binding:"required"`
		Amount   int64  `json:"amount" binding:"required"`
		Currency string `json:"currency"`
		Reason   string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "amount (nonzero) and reason are required"})
		return
	}

	target, err := uuid.Parse(req.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	if req.Amount > 0 {
		err = cc.service.Grant(c.Request.Context(), s, target, req.Amount, req.Currency, req.Reason, idem, &actor, nil)
	} else {
		err = cc.service.Debit(c.Request.Context(), s, target, -req.Amount, req.Currency, req.Reason, idem, &actor)
	}

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusCreated)
}

// Hold handles POST /store-credit/holds, reserving store credit for pending order payments.
func (cc *Controller) Hold(c *gin.Context) {
	s, u, err := storeAndUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	idem := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idem == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Idempotency-Key header is required"})
		return
	}

	var req struct {
		Amount   int64      `json:"amount" binding:"required,gt=0"`
		Currency string     `json:"currency"`
		OrderID  *uuid.UUID `json:"order_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: amount (>0) is required"})
		return
	}

	if err := cc.service.Hold(c.Request.Context(), s, u, req.Amount, req.Currency, idem, req.OrderID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

// Release handles POST /store-credit/releases, unholding previously reserved store credit upon order cancellation.
func (cc *Controller) Release(c *gin.Context) {
	s, u, err := storeAndUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	idem := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idem == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Idempotency-Key header is required"})
		return
	}

	var req struct {
		Amount   int64      `json:"amount" binding:"required,gt=0"`
		Currency string     `json:"currency"`
		OrderID  *uuid.UUID `json:"order_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: amount (>0) is required"})
		return
	}

	if err := cc.service.Release(c.Request.Context(), s, u, req.Amount, req.Currency, idem, req.OrderID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

// Reconcile handles GET /store-credit/reconcile, verifying cached account balances against underlying ledger entries.
func (cc *Controller) Reconcile(c *gin.Context) {
	s, actor, err := storeAndUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !cc.isAllowedViewer(c, s, actor) {
		c.JSON(http.StatusForbidden, gin.H{"error": "manager, admin, or owner access required"})
		return
	}

	raw := c.Query("user_id")
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}
	target, err := uuid.Parse(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	expAvail, expHeld, actAvail, actHeld, matches, err := cc.service.AuditReconcile(c.Request.Context(), s, target)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"matches":            matches,
		"expected_available": expAvail,
		"expected_held":      expHeld,
		"actual_available":   actAvail,
		"actual_held":        actHeld,
	})
}
