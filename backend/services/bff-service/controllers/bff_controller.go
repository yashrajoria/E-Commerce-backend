package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"bff-service/clients"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// pollInterval / pollTimeout control how long BFF waits for the async
// SQS consumer to create the payment record + Stripe checkout session.
const (
	checkoutPollInterval = 700 * time.Millisecond
	checkoutPollTimeout  = 35 * time.Second
)

type BFFController struct {
	gateway     *clients.GatewayClient
	redisClient *redis.Client
}

func NewBFFController(gateway *clients.GatewayClient, redisClient *redis.Client) *BFFController {
	return &BFFController{gateway: gateway, redisClient: redisClient}
}

func (b *BFFController) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (b *BFFController) Home(c *gin.Context) {
	ctx := c.Request.Context()

	productsQuery := url.Values{}
	for key, values := range c.Request.URL.Query() {
		for _, v := range values {
			productsQuery.Add(key, v)
		}
	}
	if productsQuery.Get("perPage") == "" {
		productsQuery.Set("perPage", "12")
	}

	type result struct {
		data interface{}
		err  error
	}

	productsCh := make(chan result, 1)
	categoriesCh := make(chan result, 1)

	go func() {
		resp, err := b.gateway.Do(ctx, http.MethodGet, "/products", productsQuery, c.Request.Header, nil)
		if err != nil {
			productsCh <- result{err: err}
			return
		}
		var data interface{}
		err = clients.DecodeJSON(resp, &data)
		productsCh <- result{data: data, err: err}
	}()

	go func() {
		resp, err := b.gateway.Do(ctx, http.MethodGet, "/categories", nil, c.Request.Header, nil)
		if err != nil {
			categoriesCh <- result{err: err}
			return
		}
		var data interface{}
		err = clients.DecodeJSON(resp, &data)
		categoriesCh <- result{data: data, err: err}
	}()

	products := <-productsCh
	categories := <-categoriesCh

	if products.err != nil || categories.err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":      "failed to load home data",
			"products":   errorString(products.err),
			"categories": errorString(categories.err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"products":   products.data,
		"categories": categories.data,
		"timestamp":  time.Now().UTC(),
	})
}

func (b *BFFController) Profile(c *gin.Context) {
	ctx := c.Request.Context()

	ordersQuery := url.Values{}
	for key, values := range c.Request.URL.Query() {
		for _, v := range values {
			ordersQuery.Add(key, v)
		}
	}

	type result struct {
		data map[string]interface{}
		err  error
	}

	profileCh := make(chan result, 1)
	ordersCh := make(chan result, 1)

	go func() {
		resp, err := b.gateway.Do(ctx, http.MethodGet, "/users/profile", nil, c.Request.Header, nil)
		if err != nil {
			profileCh <- result{err: err}
			return
		}
		var data map[string]interface{}
		err = clients.DecodeJSON(resp, &data)
		profileCh <- result{data: data, err: err}
	}()

	go func() {
		resp, err := b.gateway.Do(ctx, http.MethodGet, "/orders", ordersQuery, c.Request.Header, nil)
		if err != nil {
			ordersCh <- result{err: err}
			return
		}
		var data map[string]interface{}
		err = clients.DecodeJSON(resp, &data)
		ordersCh <- result{data: data, err: err}
	}()

	profile := <-profileCh
	orders := <-ordersCh

	if profile.err != nil || orders.err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "failed to load profile data",
			"profile": errorString(profile.err),
			"orders":  errorString(orders.err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"profile":   profile.data,
		"orders":    orders.data,
		"timestamp": time.Now().UTC(),
	})
}

func (b *BFFController) Proxy(method, path string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ensure backward-compatibility: if downstream expects X-User-ID
		// but the frontend now provides a cookie, copy it into the header.
		ensureUserIDHeader(c)

		bodyBytes, err := clients.ReadJSONBody(c.Request)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		resp, err := b.gateway.Do(c.Request.Context(), method, path, c.Request.URL.Query(), c.Request.Header, clients.BodyFromBytes(bodyBytes))
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "upstream request failed"})
			return
		}

		if err := clients.CopyResponse(c.Writer, resp); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read upstream response"})
			return
		}
	}
}

func (b *BFFController) OrderByID(c *gin.Context) {
	orderID := c.Param("id")
	path := "/orders/" + orderID

	resp, err := b.gateway.Do(c.Request.Context(), http.MethodGet, path, c.Request.URL.Query(), c.Request.Header, nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream request failed"})
		return
	}

	if err := clients.CopyResponse(c.Writer, resp); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read upstream response"})
		return
	}
}

func (b *BFFController) ProductByID(c *gin.Context) {
	productID := c.Param("id")
	path := "/products/" + productID

	resp, err := b.gateway.Do(c.Request.Context(), http.MethodGet, path, c.Request.URL.Query(), c.Request.Header, nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream request failed"})
		return
	}

	if err := clients.CopyResponse(c.Writer, resp); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read upstream response"})
		return
	}
}

func (b *BFFController) CartRemoveItem(c *gin.Context) {
	productID := c.Param("product_id")
	path := "/cart/remove/" + productID

	resp, err := b.gateway.Do(c.Request.Context(), http.MethodDelete, path, c.Request.URL.Query(), c.Request.Header, nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream request failed"})
		return
	}

	if err := clients.CopyResponse(c.Writer, resp); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read upstream response"})
		return
	}
}

func (b *BFFController) PaymentStatusByOrderID(c *gin.Context) {
	orderID := c.Param("order_id")
	path := "/payment/status/by-order/" + orderID

	resp, err := b.gateway.Do(c.Request.Context(), http.MethodGet, path, c.Request.URL.Query(), c.Request.Header, nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream request failed"})
		return
	}

	if err := clients.CopyResponse(c.Writer, resp); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read upstream response"})
		return
	}
}

// Checkout orchestrates:
//  1. Validate cart is non-empty.
//  2. Publish checkout event via POST /cart/checkout (returns order_id immediately).
//  3. Poll GET /payment/status/by-order/{order_id} until the async SQS consumer
//     has created the payment record AND the Stripe checkout session, then return
//     { order_id, session_id, checkout_url }.
//
// The Stripe session is created by the payment-service SQS consumer, so we must
// wait for it rather than calling /payment/create-checkout directly (which would
// race against the async consumer and always return 404).
func (b *BFFController) Checkout(c *gin.Context) {
	ctx := c.Request.Context()

	// ensure downstream services receive a user identifier when the
	// frontend supplies it via cookie instead of the X-User-ID header.
	ensureUserIDHeader(c)

	// Idempotency key required to avoid duplicate orders
	idemKey := c.GetHeader("Idempotency-Key")
	if idemKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing Idempotency-Key header"})
		return
	}
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing user identity"})
		return
	}
	cacheKey := "idem:bff:" + userID + ":" + idemKey

	if b.redisClient != nil {
		// FIX: Use SetNX as an atomic lock from the very start.
		// This prevents a race condition where two concurrent requests with the same
		// idempotency key both read a missing cache entry and both proceed to create
		// duplicate orders. SetNX guarantees only the first request acquires the lock.
		//
		// Sentinel value "pending" signals that a checkout is in-flight.
		// Once complete, the sentinel is replaced with the full JSON response.
		acquired, err := b.redisClient.SetNX(ctx, cacheKey, "pending", 15*time.Minute).Result()
		if err == nil && !acquired {
			// Key already exists — check if it holds a completed response or is still pending.
			val, getErr := b.redisClient.Get(ctx, cacheKey).Result()
			if getErr == nil {
				if val != "pending" {
					// A previous request already completed — return the cached result.
					c.Data(http.StatusOK, "application/json", []byte(val))
					return
				}
				// Still pending: another request is in-flight. Ask the client to retry.
				c.JSON(http.StatusConflict, gin.H{
					"error":   "checkout is already in progress for this idempotency key",
					"message": "please retry in a few seconds",
				})
				return
			}
		}
		// If Redis is unavailable (err != nil) we fall through and allow the request.
	}

	// 1) Validate cart is non-empty
	cartRespCheck, err := b.gateway.Do(ctx, http.MethodGet, "/cart", nil, c.Request.Header, nil)
	if err != nil {
		b.releasePendingKey(ctx, cacheKey)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch cart"})
		return
	}
	var cartCheck map[string]interface{}
	if err := clients.DecodeJSON(cartRespCheck, &cartCheck); err != nil {
		b.releasePendingKey(ctx, cacheKey)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to decode cart"})
		return
	}
	items, exists := cartCheck["items"]
	if !exists || items == nil {
		b.releasePendingKey(ctx, cacheKey)
		c.JSON(http.StatusBadRequest, gin.H{"error": "cart is empty"})
		return
	}
	if arr, ok := items.([]interface{}); ok && len(arr) == 0 {
		b.releasePendingKey(ctx, cacheKey)
		c.JSON(http.StatusBadRequest, gin.H{"error": "cart is empty"})
		return
	}

	// 2) Publish checkout event — cart-service returns { order_id } immediately.
	//    The payment record + Stripe checkout session are created asynchronously by
	//    the order-service and payment-service SQS consumers.
	checkoutResp, err := b.gateway.Do(ctx, http.MethodPost, "/cart/checkout", nil, c.Request.Header, nil)
	if err != nil {
		b.releasePendingKey(ctx, cacheKey)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to create order"})
		return
	}
	var cartResp struct {
		OrderID string `json:"order_id"`
		Status  string `json:"status"`
	}
	if err := clients.DecodeJSON(checkoutResp, &cartResp); err != nil || cartResp.OrderID == "" {
		b.releasePendingKey(ctx, cacheKey)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to decode order response"})
		return
	}

	// 3) Poll payment status until checkout_url is ready (set by SQS consumer).
	type payStatus struct {
		OrderID     string  `json:"order_id"`
		Status      string  `json:"status"`
		CheckoutURL *string `json:"checkout_url"`
		SessionID   *string `json:"session_id"`
	}
	deadline := time.Now().Add(checkoutPollTimeout)
	var finalStatus payStatus
	for time.Now().Before(deadline) {
		statusResp, err := b.gateway.Do(ctx, http.MethodGet,
			"/payment/status/by-order/"+cartResp.OrderID, nil, c.Request.Header, nil)
		if err == nil {
			var ps payStatus
			if decErr := clients.DecodeJSON(statusResp, &ps); decErr == nil {
				if ps.CheckoutURL != nil && *ps.CheckoutURL != "" {
					finalStatus = ps
					break
				}
			}
		}
		select {
		case <-ctx.Done():
			b.releasePendingKey(ctx, cacheKey)
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": "request cancelled"})
			return
		case <-time.After(checkoutPollInterval):
		}
	}

	// Timeout — try calling payment/create-checkout directly as a fallback.
	// This handles the case where the SQS consumer created the payment record but
	// Stripe session creation failed, leaving checkout_url unset.
	if finalStatus.CheckoutURL == nil || *finalStatus.CheckoutURL == "" {
		bodyPayload, _ := json.Marshal(map[string]string{"order_id": cartResp.OrderID})
		createResp, err := b.gateway.Do(ctx, http.MethodPost, "/payment/create-checkout",
			nil, c.Request.Header, clients.BodyFromBytes(bodyPayload))
		if err == nil {
			var createResult struct {
				CheckoutURL string `json:"checkout_url"`
				SessionID   string `json:"session_id"`
			}
			if decErr := clients.DecodeJSON(createResp, &createResult); decErr == nil && createResult.CheckoutURL != "" {
				out, _ := json.Marshal(map[string]string{
					"order_id":     cartResp.OrderID,
					"session_id":   createResult.SessionID,
					"checkout_url": createResult.CheckoutURL,
				})
				b.cacheCheckoutResult(ctx, cacheKey, out)
				c.Data(http.StatusOK, "application/json", out)
				return
			}
		}

		// All attempts exhausted — release the pending lock so the client can retry,
		// and return 202 so the frontend can poll for the payment status.
		b.releasePendingKey(ctx, cacheKey)
		c.JSON(http.StatusAccepted, gin.H{
			"order_id":     cartResp.OrderID,
			"status":       "PENDING_PAYMENT",
			"checkout_url": nil,
			"message":      "checkout session is being prepared; poll GET /payment/status/by-order/" + cartResp.OrderID,
		})
		return
	}

	sessionID := ""
	if finalStatus.SessionID != nil {
		sessionID = *finalStatus.SessionID
	}

	out, _ := json.Marshal(map[string]string{
		"order_id":     cartResp.OrderID,
		"session_id":   sessionID,
		"checkout_url": *finalStatus.CheckoutURL,
	})

	// Replace the "pending" sentinel with the final response so future duplicate
	// requests return the cached result immediately.
	b.cacheCheckoutResult(ctx, cacheKey, out)

	c.Data(http.StatusOK, "application/json", out)
}

// releasePendingKey deletes the idempotency "pending" sentinel from Redis on error
// paths so the client can safely retry the checkout without getting a 409.
// It is a no-op when Redis is unavailable or the key was never set.
func (b *BFFController) releasePendingKey(ctx context.Context, cacheKey string) {
	if b.redisClient == nil {
		return
	}
	// Only delete if still "pending" — don't accidentally wipe a completed entry
	// set by a concurrent request that succeeded slightly ahead of us.
	val, err := b.redisClient.Get(ctx, cacheKey).Result()
	if err == nil && val == "pending" {
		b.redisClient.Del(ctx, cacheKey)
	}
}

// cacheCheckoutResult overwrites the "pending" sentinel with the final JSON payload.
// Uses Set (not SetNX) so we always replace the sentinel even if it has expired and
// been re-acquired by a concurrent request.
func (b *BFFController) cacheCheckoutResult(ctx context.Context, cacheKey string, data []byte) {
	if b.redisClient == nil {
		return
	}
	_ = b.redisClient.Set(ctx, cacheKey, data, 15*time.Minute).Err()
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// ensureUserIDHeader copies the `user_id` cookie into the `X-User-ID` header
// when the header is missing. This provides a small compatibility shim while
// services migrate from header-based identity to cookie/session-based auth.
func ensureUserIDHeader(c *gin.Context) {
	if c.GetHeader("X-User-ID") == "" {
		if cookie, err := c.Request.Cookie("user_id"); err == nil && cookie != nil {
			c.Request.Header.Set("X-User-ID", cookie.Value)
		}
	}
}
