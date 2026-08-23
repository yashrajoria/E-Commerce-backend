package services

import (
	"context"
	"encoding/json"
	"log"
	"order-service/models"
	repositories "order-service/repository"
	"time"

	"github.com/google/uuid"
	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"github.com/yashrajoria/common/events"
)

type SQSCheckoutConsumer struct {
	sqsConsumer          *aws_pkg.SQSConsumer
	sqsPublisher         *aws_pkg.SQSConsumer // For sending payment requests
	orderRepo            repositories.OrderRepository
	inventoryClient      *InventoryClient
	metricsClient        *aws_pkg.MetricsClient
	productServiceURL    string // Base URL for the product service (internal endpoint)
	snsClient            aws_pkg.SNSPublisher
	notificationTopicArn string
	promotionClient      *PromotionClient
	storeCurrency        string
}

// NewSQSCheckoutConsumer creates a new SQS-based checkout consumer
func NewSQSCheckoutConsumer(sqsConsumer *aws_pkg.SQSConsumer, sqsPublisher *aws_pkg.SQSConsumer, orderRepo repositories.OrderRepository, inventoryClient *InventoryClient, metricsClient *aws_pkg.MetricsClient, productServiceURL string, snsClient aws_pkg.SNSPublisher, notificationTopicArn string, promotionClient *PromotionClient, storeCurrency string) *SQSCheckoutConsumer {
	if productServiceURL == "" {
		productServiceURL = "http://product-service:8082"
	}
	if storeCurrency == "" {
		storeCurrency = "usd"
	}
	return &SQSCheckoutConsumer{
		sqsConsumer:          sqsConsumer,
		sqsPublisher:         sqsPublisher,
		orderRepo:            orderRepo,
		inventoryClient:      inventoryClient,
		metricsClient:        metricsClient,
		productServiceURL:    productServiceURL,
		snsClient:            snsClient,
		notificationTopicArn: notificationTopicArn,
		promotionClient:      promotionClient,
		storeCurrency:        storeCurrency,
	}
}

// Start begins polling the checkout queue
func (c *SQSCheckoutConsumer) Start(ctx context.Context) {
	log.Println("[OrderService][SQSCheckoutConsumer] Starting checkout queue consumer")

	err := c.sqsConsumer.StartPolling(ctx, func(ctx context.Context, body string) error {
		return c.handleMessage(ctx, body)
	})
	if err != nil && err != context.Canceled {
		log.Printf("❌ [OrderService][SQSCheckoutConsumer] polling error: %v", err)
	}
}

func (c *SQSCheckoutConsumer) handleMessage(ctx context.Context, body string) error {
	log.Printf("[DEBUG] Raw SQS message: %s", body)

	// Try to unwrap SNS envelope if present
	var snsEnvelope struct {
		Message string `json:"Message"`
	}
	if err := json.Unmarshal([]byte(body), &snsEnvelope); err == nil && snsEnvelope.Message != "" {
		body = snsEnvelope.Message
	}

	var evt models.CheckoutEvent
	if err := json.Unmarshal([]byte(body), &evt); err != nil {
		log.Printf("❌ invalid JSON: %v payload=%s", err, body)
		return nil // Don't retry invalid JSON
	}

	userUUID, err := uuid.Parse(evt.UserID)
	if err != nil {
		log.Printf("❌ user_id is not a valid UUID: %s", evt.UserID)
		return nil
	}

	if evt.OrderID == "" {
		log.Printf("❌ missing OrderID in CheckoutEvent, skipping")
		return nil
	}
	orderIDUUID, err := uuid.Parse(evt.OrderID)
	if err != nil {
		log.Printf("❌ invalid OrderID UUID format: %s", evt.OrderID)
		return nil
	}

	// Idempotency: if CheckoutEvent contains an idempotency key, check DB for existing order
	if evt.IdempotencyKey != "" {
		if existing, err := c.orderRepo.FindByIdempotencyKey(ctx, evt.IdempotencyKey); err == nil {
			log.Printf("⚠️  [IDEMPOTENCY] Order already exists for key=%s order_id=%s user=%s (skipping creation)", evt.IdempotencyKey, existing.ID.String(), existing.UserID.String())
			return nil
		}
	}

	orderItems := make([]models.OrderItem, 0, len(evt.Items))
	totalAmount := 0
	validItems := 0
	productServiceURL := c.productServiceURL
	inventoryItems := make([]ReserveItem, 0, len(evt.Items))
	notificationItems := make([]events.NotificationItem, 0, len(evt.Items))

	for _, it := range evt.Items {
		pid, err := uuid.Parse(it.ProductID)
		if err != nil {
			log.Printf("⚠️ skipping item with invalid product_id=%s", it.ProductID)
			continue
		}

		if it.Quantity <= 0 {
			log.Printf("⚠️ skipping item with invalid quantity product_id=%s qty=%d", it.ProductID, it.Quantity)
			continue
		}

		product, err := FetchProductByID(ctx, productServiceURL, pid)
		if err != nil {
			log.Printf("⚠️ failed to fetch product for product_id=%s: %v", it.ProductID, err)
			continue
		}

		orderItem := models.OrderItem{
			ID:        uuid.New(),
			ProductID: pid,
			Quantity:  it.Quantity,
			Price:     int(product.Price),
		}

		totalAmount += it.Quantity * int(product.Price)
		orderItems = append(orderItems, orderItem)
		inventoryItems = append(inventoryItems, ReserveItem{
			ProductID: it.ProductID,
			Quantity:  it.Quantity,
		})
		notificationItems = append(notificationItems, events.NotificationItem{
			ProductName: product.Name,
			Quantity:    it.Quantity,
			Price:       product.Price,
		})
		validItems++
	}

	if validItems == 0 {
		log.Printf("❌ [CHECKOUT] No valid items for user=%s order=%s, aborting order creation", evt.UserID, evt.OrderID)
		return nil // Don't retry - this is a data issue, not a transient error
	}

	// Process Coupon if present
	discountAmount := 0
	if evt.CouponCode != "" && c.promotionClient != nil {
		resp, err := c.promotionClient.ValidateCoupon(ctx, evt.CouponCode, evt.UserID, float64(totalAmount))
		if err != nil {
			log.Printf("⚠️  [CHECKOUT] Coupon validation failed for code=%s: %v", evt.CouponCode, err)
		} else if resp != nil && resp.Valid {
			discountAmount = int(resp.DiscountAmount)
			log.Printf("✅ [CHECKOUT] Coupon applied: code=%s discount=%d", evt.CouponCode, discountAmount)
		} else {
			log.Printf("⚠️  [CHECKOUT] Coupon invalid: code=%s", evt.CouponCode)
		}
	}

	finalTotal := totalAmount - discountAmount
	if finalTotal < 0 {
		finalTotal = 0
	}

	// Reserve inventory via inventory service before creating the order/payment request
	if c.inventoryClient != nil {
		if err := c.inventoryClient.ReserveStock(ctx, orderIDUUID.String(), inventoryItems); err != nil {
			// Do not create an order or request payment unless stock is actually reserved.
			// Returning an error lets SQS retry transient inventory failures instead of
			// producing paid orders that cannot be fulfilled.
			log.Printf("❌ [CHECKOUT] Inventory reservation failed for order=%s: %v", orderIDUUID.String(), err)
			return err
		} else {
			log.Printf("✅ [CHECKOUT] Inventory reserved for order=%s items=%d", orderIDUUID.String(), len(inventoryItems))
		}
	} else {
		log.Printf("⚠️  [CHECKOUT] Inventory client not configured, skipping reservation")
	}

	order := models.Order{
		UserID:         userUUID,
		ID:             orderIDUUID,
		Amount:         finalTotal,
		CouponCode:     evt.CouponCode,
		DiscountAmount: discountAmount,
		Status:         "pending_payment",
		OrderNumber:    "ORD-" + time.Now().Format("20060102-150405") + "-" + uuid.New().String()[:8],
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		OrderItems:     orderItems,
	}
	if evt.IdempotencyKey != "" {
		order.IdempotencyKey = &evt.IdempotencyKey
	}

	if err := c.orderRepo.Create(ctx, &order); err != nil {
		log.Printf("❌ [CHECKOUT] Failed to create order record (will retry): %v", err)
		// Compensate: release the stock we just reserved so it doesn't stay locked
		// indefinitely if this message eventually dead-letters. Safe to re-reserve
		// on retry since ReserveStock is idempotent via ClientRequestToken.
		if c.inventoryClient != nil {
			if relErr := c.inventoryClient.ReleaseStock(ctx, orderIDUUID.String(), inventoryItems); relErr != nil {
				log.Printf("❌ [CHECKOUT] Failed to release reserved stock after order create failure for order=%s: %v", orderIDUUID.String(), relErr)
			} else {
				log.Printf("↩️  [CHECKOUT] Released reserved stock after order create failure for order=%s", orderIDUUID.String())
			}
		}
		return err
	}

	log.Printf("✅ order created id=%s user=%s items=%d total_amount=%d",
		order.ID.String(), order.UserID.String(), validItems, order.Amount)

	// Publish order_created notification with correct total and items
	if c.snsClient != nil && c.notificationTopicArn != "" {
		notifEvent := events.NewOrderCreatedEvent(
			evt.UserID, evt.Email, "", "", order.ID.String(), order.CouponCode,
			float64(order.Amount), notificationItems,
		)
		notifBytes, err := json.Marshal(notifEvent)
		if err != nil {
			log.Printf("⚠️ failed to marshal order_created notification: %v", err)
		} else if err := c.snsClient.Publish(ctx, c.notificationTopicArn, notifBytes); err != nil {
			log.Printf("⚠️ failed to publish order_created notification: %v", err)
		} else {
			log.Printf("✅ order_created notification published for order=%s", order.ID.String())
		}
	}

	// Emit metrics
	if c.metricsClient != nil && c.metricsClient.IsEnabled() {
		go func() {
			metricCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			dims := map[string]string{"Service": "order-service"}
			_ = c.metricsClient.RecordCount(metricCtx, aws_pkg.MetricOrdersCreated, dims)
			_ = c.metricsClient.RecordValue(metricCtx, "OrderAmount", float64(order.Amount), dims)
		}()
	}

	// Send payment request to SQS
	idemKey := evt.IdempotencyKey
	if idemKey == "" {
		idemKey = order.ID.String()
	}

	req := models.PaymentRequest{
		OrderID:        order.ID.String(),
		UserID:         order.UserID.String(),
		Email:          evt.Email,
		Amount:         order.Amount,
		Currency:       c.storeCurrency,
		IdempotencyKey: idemKey,
	}
	reqBytes, _ := json.Marshal(req)
	if err := c.sqsPublisher.SendMessage(ctx, string(reqBytes)); err != nil {
		log.Printf("❌ failed to publish payment-request for order=%s: %v", order.ID.String(), err)
		// Don't return error - order is created, payment request can be retried
	} else {
		log.Printf("✅ payment-request sent for order=%s", order.ID.String())
	}

	return nil
}
