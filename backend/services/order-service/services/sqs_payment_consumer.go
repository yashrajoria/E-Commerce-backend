package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"order-service/models"
	repositories "order-service/repository"
	"time"
	"github.com/google/uuid"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"github.com/yashrajoria/common/events"
)

// SQSPaymentConsumer consumes payment events from SQS and updates order status
type SQSPaymentConsumer struct {
	sqsConsumer          *aws_pkg.SQSConsumer
	orderRepo            repositories.OrderRepository
	inventoryClient      *InventoryClient
	metricsClient        *aws_pkg.MetricsClient
	snsClient            aws_pkg.SNSPublisher
	notificationTopicArn string
	productServiceURL    string
}

// NewSQSPaymentConsumer creates a new SQS-based payment event consumer
func NewSQSPaymentConsumer(sqsConsumer *aws_pkg.SQSConsumer, orderRepo repositories.OrderRepository, inventoryClient *InventoryClient, metricsClient *aws_pkg.MetricsClient, snsClient aws_pkg.SNSPublisher, notificationTopicArn string, productServiceURL string) *SQSPaymentConsumer {
	return &SQSPaymentConsumer{
		sqsConsumer:          sqsConsumer,
		orderRepo:            orderRepo,
		inventoryClient:      inventoryClient,
		metricsClient:        metricsClient,
		snsClient:            snsClient,
		notificationTopicArn: notificationTopicArn,
		productServiceURL:    productServiceURL,
	}
}

// Start begins polling the payment events queue
func (c *SQSPaymentConsumer) Start(ctx context.Context) {
	log.Println("[OrderService][SQSPaymentConsumer] Starting payment events queue consumer")

	err := c.sqsConsumer.StartPolling(ctx, func(ctx context.Context, body string) error {
		return c.handleMessage(ctx, body)
	})
	if err != nil && err != context.Canceled {
		log.Printf("❌ [OrderService][SQSPaymentConsumer] polling error: %v", err)
	}
}

func (c *SQSPaymentConsumer) handleMessage(ctx context.Context, body string) error {
	log.Printf("[DEBUG] Raw payment event: %s", body)

	// Try to unwrap SNS envelope if present
	var snsEnvelope struct {
		Message string `json:"Message"`
	}
	if err := json.Unmarshal([]byte(body), &snsEnvelope); err == nil && snsEnvelope.Message != "" {
		body = snsEnvelope.Message
	}

	var evt models.PaymentEvent
	if err := json.Unmarshal([]byte(body), &evt); err != nil {
		log.Printf("❌ [OrderService][SQSPaymentConsumer] invalid JSON: %v payload=%s", err, body)
		return nil // Don't retry invalid JSON
	}

	if evt.OrderID == "" || evt.Type == "" {
		log.Printf("❌ [OrderService][SQSPaymentConsumer] missing fields: order_id=%q type=%q", evt.OrderID, evt.Type)
		return nil
	}

	log.Printf("ℹ️  [OrderService][SQSPaymentConsumer] received event: order_id=%s type=%s", evt.OrderID, evt.Type)

	now := time.Now()
	switch evt.Type {
	case "payment_succeeded":
		// Gate side effects on the update actually applying: a duplicate/redelivered
		// event that loses the atomic status race must not re-confirm inventory or
		// re-send the order_confirmed email.
		if c.updateOrderStatusWithTime(ctx, evt.OrderID, "paid", &now, nil) {
			c.confirmInventory(ctx, evt.OrderID)
			c.publishOrderConfirmedNotification(ctx, evt)
			if c.metricsClient != nil && c.metricsClient.IsEnabled() {
				go func() {
					metricCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					dims := map[string]string{"Service": "order-service"}
					_ = c.metricsClient.RecordCount(metricCtx, aws_pkg.MetricPaymentSucceeded, dims)
					_ = c.metricsClient.RecordCount(metricCtx, aws_pkg.MetricOrdersCompleted, dims)
				}()
			}
		}
	case "payment_failed":
		if c.updateOrderStatusWithTime(ctx, evt.OrderID, "payment_failed", nil, &now) {
			c.releaseInventory(ctx, evt.OrderID)
			if c.metricsClient != nil && c.metricsClient.IsEnabled() {
				go func() {
					metricCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					dims := map[string]string{"Service": "order-service"}
					_ = c.metricsClient.RecordCount(metricCtx, aws_pkg.MetricPaymentFailed, dims)
					_ = c.metricsClient.RecordCount(metricCtx, aws_pkg.MetricOrdersFailed, dims)
				}()
			}
		}
	case "checkout_session_created":
		log.Printf("ℹ️  [OrderService][SQSPaymentConsumer] checkout session created for order=%s", evt.OrderID)
	case "checkout_session_failed":
		if c.updateOrderStatusWithTime(ctx, evt.OrderID, "payment_failed", nil, &now) {
			c.releaseInventory(ctx, evt.OrderID)
		}
	default:
		log.Printf("⚠️  [OrderService][SQSPaymentConsumer] unknown event type: %s", evt.Type)
	}

	return nil
}

// updateOrderStatusWithTime transitions an order to status using an atomic
// conditional UPDATE (status = 'pending_payment' -> status) instead of a
// read-modify-write, so concurrent/duplicate payment webhooks can't race each
// other into an inconsistent status. Returns true only when this call performed
// the transition — callers must gate side effects (inventory confirm/release,
// notification emails, metrics) on this so a duplicate delivery that loses the
// race doesn't repeat them.
func (c *SQSPaymentConsumer) updateOrderStatusWithTime(ctx context.Context, orderID string, status string, completedAt, canceledAt *time.Time) bool {
	orderUUID, err := uuid.Parse(orderID)
	if err != nil {
		log.Printf("❌ [OrderService][SQSPaymentConsumer] invalid order ID: %s", orderID)
		return false
	}

	extra := map[string]interface{}{}
	if completedAt != nil {
		extra["completed_at"] = completedAt
	}
	if canceledAt != nil {
		extra["canceled_at"] = canceledAt
	}

	err = c.orderRepo.UpdateOrderStatus(ctx, orderUUID, "pending_payment", status, extra)
	if err == nil {
		log.Printf("✅ [OrderService][SQSPaymentConsumer] order=%s updated to %s", orderID, status)
		return true
	}
	if err != repositories.ErrStatusConflict {
		log.Printf("❌ [OrderService][SQSPaymentConsumer] failed to update order=%s: %v", orderID, err)
		return false
	}

	// Status didn't match "pending_payment" — either a duplicate delivery of
	// this same event (already applied) or the order is in some other
	// terminal state. Either way, don't overwrite it; just log which.
	order, ferr := c.orderRepo.FindByID(ctx, orderUUID)
	if ferr != nil {
		log.Printf("❌ [OrderService][SQSPaymentConsumer] failed to find order=%s after status conflict: %v", orderID, ferr)
		return false
	}
	if order.Status == status {
		log.Printf("ℹ️  [OrderService][SQSPaymentConsumer] order=%s already %s; skipping", orderID, status)
		return false
	}
	log.Printf("⚠️  [OrderService][SQSPaymentConsumer] order=%s status conflict: current=%s target=%s; skipping", orderID, order.Status, status)
	return false
}

// loadOrderItems fetches order items from the DB for inventory operations
func (c *SQSPaymentConsumer) loadOrderItems(ctx context.Context, orderID string) ([]ReserveItem, error) {
	orderUUID, err := uuid.Parse(orderID)
	if err != nil {
		return nil, err
	}
	order, err := c.orderRepo.FindByID(ctx, orderUUID)
	if err != nil {
		return nil, err
	}
	result := make([]ReserveItem, len(order.OrderItems))
	for i, it := range order.OrderItems {
		result[i] = ReserveItem{
			ProductID: it.ProductID.String(),
			Quantity:  it.Quantity,
		}
	}
	return result, nil
}

// confirmInventory confirms reserved stock after successful payment
func (c *SQSPaymentConsumer) confirmInventory(ctx context.Context, orderID string) {
	items, err := c.loadOrderItems(ctx, orderID)
	if err != nil {
		log.Printf("❌ [OrderService][SQSPaymentConsumer] failed to load order items for confirm: order=%s err=%v", orderID, err)
		return
	}
	if len(items) == 0 {
		return
	}
	if err := c.inventoryClient.ConfirmStock(ctx, orderID, items); err != nil {
		log.Printf("❌ [OrderService][SQSPaymentConsumer] inventory confirm failed: order=%s err=%v", orderID, err)
	} else {
		log.Printf("✅ [OrderService][SQSPaymentConsumer] inventory confirmed for order=%s", orderID)
	}
}

// releaseInventory releases reserved stock after payment failure
func (c *SQSPaymentConsumer) releaseInventory(ctx context.Context, orderID string) {
	items, err := c.loadOrderItems(ctx, orderID)
	if err != nil {
		log.Printf("❌ [OrderService][SQSPaymentConsumer] failed to load order items for release: order=%s err=%v", orderID, err)
		return
	}
	if len(items) == 0 {
		return
	}
	if err := c.inventoryClient.ReleaseStock(ctx, orderID, items); err != nil {
		log.Printf("❌ [OrderService][SQSPaymentConsumer] inventory release failed: order=%s err=%v", orderID, err)
	} else {
		log.Printf("✅ [OrderService][SQSPaymentConsumer] inventory released for order=%s", orderID)
	}
}

// publishOrderConfirmedNotification sends an order_confirmed notification with product details
// after a successful payment, so the user receives an email with what they purchased.
func (c *SQSPaymentConsumer) publishOrderConfirmedNotification(ctx context.Context, evt models.PaymentEvent) {
	if c.snsClient == nil || c.notificationTopicArn == "" {
		log.Printf("⚠️ [OrderService][SQSPaymentConsumer] SNS not configured, skipping order_confirmed notification")
		return
	}

	// Load the order to get total amount and user info
	orderUUID, err := uuid.Parse(evt.OrderID)
	if err != nil {
		log.Printf("⚠️ [OrderService][SQSPaymentConsumer] invalid order ID in notification: %s", evt.OrderID)
		return
	}
	order, err := c.orderRepo.FindByID(ctx, orderUUID)
	if err != nil {
		log.Printf("⚠️ [OrderService][SQSPaymentConsumer] failed to load order for notification: order=%s err=%v", evt.OrderID, err)
		return
	}

	// Fetch product names for each order item
	productServiceURL := c.productServiceURL
	if productServiceURL == "" {
		productServiceURL = "http://product-service:8082"
	}

	notifItems := make([]events.NotificationItem, 0, len(order.OrderItems))
	for _, oi := range order.OrderItems {
		product, err := FetchProductByID(ctx, productServiceURL, oi.ProductID)
		name := fmt.Sprintf("Product %s", oi.ProductID.String()[:8])
		price := float64(oi.Price)
		if err == nil && product.Name != "" {
			name = product.Name
			price = product.Price
		}
		notifItems = append(notifItems, events.NotificationItem{
			ProductName: name,
			Quantity:    oi.Quantity,
			Price:       price,
		})
	}

	// Use email propagated from payment-service so notification-service can route
	// order_confirmed events to the recipient address.
	notifEvent := events.NewOrderConfirmedEvent(
		evt.UserID, evt.Email, "", evt.OrderID, float64(order.Amount), notifItems,
	)
	notifBytes, err := json.Marshal(notifEvent)
	if err != nil {
		log.Printf("⚠️ [OrderService][SQSPaymentConsumer] failed to marshal order_confirmed notification: %v", err)
		return
	}
	if err := c.snsClient.Publish(ctx, c.notificationTopicArn, notifBytes); err != nil {
		log.Printf("⚠️ [OrderService][SQSPaymentConsumer] failed to publish order_confirmed notification: %v", err)
	} else {
		log.Printf("✅ [OrderService][SQSPaymentConsumer] order_confirmed notification published for order=%s", evt.OrderID)
	}
}
