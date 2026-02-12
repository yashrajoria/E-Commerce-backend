package services

import (
	"context"
	"encoding/json"
	"log"
	"order-service/models"
	"os"
	"time"

	"github.com/google/uuid"
	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"gorm.io/gorm"
)

// SQSCheckoutConsumer consumes checkout events from SQS and creates orders
type SQSCheckoutConsumer struct {
	sqsConsumer    *aws_pkg.SQSConsumer
	sqsPublisher   *aws_pkg.SQSConsumer // For sending payment requests
	db             *gorm.DB
}

// NewSQSCheckoutConsumer creates a new SQS-based checkout consumer
func NewSQSCheckoutConsumer(sqsConsumer *aws_pkg.SQSConsumer, sqsPublisher *aws_pkg.SQSConsumer, db *gorm.DB) *SQSCheckoutConsumer {
	return &SQSCheckoutConsumer{
		sqsConsumer:  sqsConsumer,
		sqsPublisher: sqsPublisher,
		db:           db,
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

	orderItems := make([]models.OrderItem, 0, len(evt.Items))
	totalAmount := 0
	validItems := 0
	productServiceURL := os.Getenv("PRODUCT_SERVICE_URL")

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

		if product.Stock < it.Quantity {
			log.Printf("⚠️ insufficient stock for product_id=%s: available=%d requested=%d", it.ProductID, product.Stock, it.Quantity)
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
		validItems++
	}

	if validItems == 0 {
		log.Printf("❌ no valid items for user=%s, skipping order", evt.UserID)
		return nil
	}

	order := models.Order{
		UserID:      userUUID,
		ID:          orderIDUUID,
		Amount:      totalAmount,
		Status:      "pending_payment",
		OrderNumber: "ORD-" + time.Now().Format("20060102-150405") + "-" + uuid.New().String()[:8],
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err = c.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		for i := range orderItems {
			orderItems[i].OrderID = order.ID
		}
		return tx.Create(&orderItems).Error
	})
	if err != nil {
		log.Printf("❌ DB transaction failed for user=%s err=%v", evt.UserID, err)
		return err // Retry
	}

	log.Printf("✅ order created id=%s user=%s items=%d total_amount=%d",
		order.ID.String(), order.UserID.String(), validItems, order.Amount)

	// Send payment request to SQS
	req := models.PaymentRequest{
		OrderID: order.ID.String(),
		UserID:  order.UserID.String(),
		Amount:  order.Amount,
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
