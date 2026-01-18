package controllers

import (
	"net/http"
	"net/url"
	"time"

	"bff-service/clients"

	"github.com/gin-gonic/gin"
)

type BFFController struct {
	gateway *clients.GatewayClient
}

func NewBFFController(gateway *clients.GatewayClient) *BFFController {
	return &BFFController{gateway: gateway}
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
		data map[string]interface{}
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
		var data map[string]interface{}
		err = clients.DecodeJSON(resp, &data)
		productsCh <- result{data: data, err: err}
	}()

	go func() {
		resp, err := b.gateway.Do(ctx, http.MethodGet, "/categories", nil, c.Request.Header, nil)
		if err != nil {
			categoriesCh <- result{err: err}
			return
		}
		var data map[string]interface{}
		err = clients.DecodeJSON(resp, &data)
		categoriesCh <- result{data: data, err: err}
	}()

	products := <-productsCh
	categories := <-categoriesCh

	if products.err != nil || categories.err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":     "failed to load home data",
			"products":  errorString(products.err),
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

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
