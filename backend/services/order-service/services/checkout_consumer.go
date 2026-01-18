package services

import (
	"context"
	"encoding/json"
	"log"
	"order-service/kafka"
	"order-service/models"
	"os"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func StartCheckoutConsumer(brokers []string, topic, groupID string, db *gorm.DB, paymentProducer *kafka.Producer) {
	if topic == "" {
		log.Fatal("❌ [OrderService][CheckoutConsumer] empty topic: set CHECKOUT_TOPIC or pass a topic name")
	}

	r := kafka_go_reader(brokers, topic, groupID)
	defer r.Close()

	log.Printf("[OrderService][CheckoutConsumer] listening topic=%s group=%s brokers=%v", topic, groupID, brokers)

	for {
		m, err := r.ReadMessage(context.Background())
		if err != nil {
			log.Printf("❌ read error: %v", err)
			continue
		}

		log.Printf("[DEBUG] Raw Kafka message: %s", string(m.Value))

		var evt models.CheckoutEvent
		if err := json.Unmarshal(m.Value, &evt); err != nil {
			log.Printf("❌ invalid JSON: %v payload=%s", err, string(m.Value))
			continue
		}

		userUUID, err := uuid.Parse(evt.UserID)
		if err != nil {
			log.Printf("❌ user_id is not a valid UUID: %s", evt.UserID)
			continue
		}

		if evt.OrderID == "" {
			log.Printf("❌ missing OrderID in CheckoutEvent, skipping")
			continue
		}
		orderID_uuid, err := uuid.Parse(evt.OrderID)
		if err != nil {
			log.Printf("❌ invalid OrderID UUID format: %s", evt.OrderID)
			continue
		}

		orderItems := make([]models.OrderItem, 0, len(evt.Items))
		totalAmount := 0
		validItems := 0
		productServiceURL := os.Getenv("PRODUCT_SERVICE_URL")

		for _, it := range evt.Items {
			// Parse product UUID
			pid, err := uuid.Parse(it.ProductID)
			if err != nil {
				log.Printf("⚠️ skipping item with invalid product_id=%s", it.ProductID)
				continue
			}

			// Skip invalid quantity
			if it.Quantity <= 0 {
				log.Printf("⚠️ skipping item with invalid quantity product_id=%s qty=%d", it.ProductID, it.Quantity)
				continue
			}

			// Fetch product details
			product, err := FetchProductByID(context.Background(), productServiceURL, pid)
			if err != nil {
				log.Printf("⚠️ failed to fetch product for product_id=%s: %v", it.ProductID, err)
				continue
			}

			productQuantity := product.Stock

			if productQuantity < it.Quantity {
				log.Printf("⚠️ insufficient stock for product_id=%s: available=%d requested=%d", it.ProductID, productQuantity, it.Quantity)
				continue
			}

			// Build order item
			orderItem := models.OrderItem{
				ID:        uuid.New(),
				ProductID: pid,
				Quantity:  it.Quantity,
				Price:     int(product.Price),
			}

			totalAmount += it.Quantity * int(product.Price)
			orderItems = append(orderItems, orderItem)

			// Update total order amount
			// totalAmount += float64(it.Quantity) * product.Price
			validItems++
		}

		if validItems == 0 {
			log.Printf("❌ no valid items for user=%s, skipping order", evt.UserID)
			continue
		}

		// Create the order
		order := models.Order{
			UserID:      userUUID,
			ID:          orderID_uuid,
			Amount:      totalAmount,
			Status:      "pending_payment",
			OrderNumber: "ORD-" + time.Now().Format("20060102-150405") + "-" + uuid.New().String()[:8],
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		// Persist order and items in a single transaction
		err = db.Transaction(func(tx *gorm.DB) error {
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
			continue
		}

		log.Printf("✅ order created id=%s user=%s items=%d total_amount=%d",
			order.ID.String(), order.UserID.String(), validItems, order.Amount)

		// Emit payment request
		req := models.PaymentRequest{
			OrderID: order.ID.String(),
			UserID:  order.UserID.String(),
			Amount:  order.Amount,
		}
		if err := paymentProducer.SendPaymentRequest(req); err != nil {
			log.Printf("❌ failed to publish payment-request for order=%s: %v", order.ID.String(), err)
			continue
		}
	}
}

// tiny wrapper to keep imports tidy / explicit naming
func kafka_go_reader(brokers []string, topic, groupID string) *kafkago.Reader {
	return kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  groupID,
		MinBytes: 1e3,
		MaxBytes: 10e6,
	})
}
