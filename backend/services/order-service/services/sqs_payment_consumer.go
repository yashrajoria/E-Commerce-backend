package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"order-service/models"
	"time"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"github.com/yashrajoria/common/events"
	"gorm.io/gorm"
)

// SQSPaymentConsumer consumes payment events from SQS and updates order status
type SQSPaymentConsumer struct {
	sqsConsumer          *aws_pkg.SQSConsumer
	db                   *gorm.DB
	inventoryClient      *InventoryClient
	metricsClient        *aws_pkg.MetricsClient
	snsClient            aws_pkg.SNSPublisher
	notificationTopicArn string
	productServiceURL    string
}

// NewSQSPaymentConsumer creates a new SQS-based payment event consumer
func NewSQSPaymentConsumer(sqsConsumer *aws_pkg.SQSConsumer, db *gorm.DB, inventoryClient *InventoryClient, metricsClient *aws_pkg.MetricsClient, snsClient aws_pkg.SNSPublisher, notificationTopicArn string, productServiceURL string) *SQSPaymentConsumer {
	return &SQSPaymentConsumer{
		sqsConsumer:          sqsConsumer,
		db:                   db,
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
		c.updateOrderStatusWithTime(evt.OrderID, "paid", &now, nil)
		c.confirmInventory(ctx, evt.OrderID)
		// Send order_confirmed notification with product details
		c.publishOrderConfirmedNotification(ctx, evt)
		// Emit metrics
		if c.metricsClient != nil && c.metricsClient.IsEnabled() {
			go func() {
				metricCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				dims := map[string]string{"Service": "order-service"}
				_ = c.metricsClient.RecordCount(metricCtx, aws_pkg.MetricPaymentSucceeded, dims)
				_ = c.metricsClient.RecordCount(metricCtx, aws_pkg.MetricOrdersCompleted, dims)
			}()
		}
	case "payment_failed":
		c.updateOrderStatusWithTime(evt.OrderID, "payment_failed", nil, &now)
		c.releaseInventory(ctx, evt.OrderID)
		// Emit metrics
		if c.metricsClient != nil && c.metricsClient.IsEnabled() {
			go func() {
				metricCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				dims := map[string]string{"Service": "order-service"}
				_ = c.metricsClient.RecordCount(metricCtx, aws_pkg.MetricPaymentFailed, dims)
				_ = c.metricsClient.RecordCount(metricCtx, aws_pkg.MetricOrdersFailed, dims)
			}()
		}
	case "checkout_session_created":
		log.Printf("ℹ️  [OrderService][SQSPaymentConsumer] checkout session created for order=%s", evt.OrderID)
	case "checkout_session_failed":
		c.updateOrderStatusWithTime(evt.OrderID, "payment_failed", nil, &now)
		c.releaseInventory(ctx, evt.OrderID)
	default:
		log.Printf("⚠️  [OrderService][SQSPaymentConsumer] unknown event type: %s", evt.Type)
	}

	return nil
}

func (c *SQSPaymentConsumer) updateOrderStatusWithTime(orderID, status string, completedAt, canceledAt *time.Time) {
	updateFields := map[string]interface{}{
		"status": status,
	}
	if completedAt != nil {
		updateFields["completed_at"] = *completedAt
	}
	if canceledAt != nil {
		updateFields["canceled_at"] = *canceledAt
	}

	err := c.db.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.First(&order, "id = ?", orderID).Error; err != nil {
			return err
		}
		if order.Status == status {
			needsUpdate := false
			if completedAt != nil && order.CompletedAt == nil {
				updateFields["completed_at"] = *completedAt
				needsUpdate = true
			}
			if canceledAt != nil && order.CanceledAt == nil {
				updateFields["canceled_at"] = *canceledAt
				needsUpdate = true
			}
			if !needsUpdate {
				log.Printf("ℹ️  [OrderService][SQSPaymentConsumer] order=%s already %s; skipping", orderID, status)
				return nil
			}
		}
		return tx.Model(&order).Updates(updateFields).Error
	})
	if err != nil {
		log.Printf("❌ [OrderService][SQSPaymentConsumer] failed to update order=%s: %v", orderID, err)
	} else {
		log.Printf("✅ [OrderService][SQSPaymentConsumer] order=%s updated to %s", orderID, status)
	}
}

// loadOrderItems fetches order items from the DB for inventory operations
func (c *SQSPaymentConsumer) loadOrderItems(orderID string) ([]ReserveItem, error) {
	var items []models.OrderItem
	if err := c.db.Where("order_id = ?", orderID).Find(&items).Error; err != nil {
		return nil, err
	}
	result := make([]ReserveItem, len(items))
	for i, it := range items {
		result[i] = ReserveItem{
			ProductID: it.ProductID.String(),
			Quantity:  it.Quantity,
		}
	}
	return result, nil
}

// confirmInventory confirms reserved stock after successful payment
func (c *SQSPaymentConsumer) confirmInventory(ctx context.Context, orderID string) {
	items, err := c.loadOrderItems(orderID)
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
	items, err := c.loadOrderItems(orderID)
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
	var order models.Order
	if err := c.db.Preload("OrderItems").First(&order, "id = ?", evt.OrderID).Error; err != nil {
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

	// evt.UserID comes from the payment event; email is not available in the payment event,
	// so we pass it as empty and rely on the notification service to use the Recipient field.
	// TODO: consider storing email in the order record for better reliability.
	notifEvent := events.NewOrderConfirmedEvent(
		evt.UserID, "", "", evt.OrderID, float64(order.Amount), notifItems,
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
