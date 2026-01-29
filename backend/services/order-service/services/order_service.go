package services

import (
	"context"
	"encoding/json"
	"log"
	"order-service/kafka"
	"order-service/models"
	repositories "order-service/repository"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"

	"time"

	"github.com/google/uuid"
)

type CreateOrderRequest struct {
	Items []struct {
		ProductID uuid.UUID `json:"product_id" binding:"required"`
		Quantity  int       `json:"quantity" binding:"required,min=1"`
	} `json:"items" binding:"required,dive"`
}

type OrderResponse struct {
	Orders []models.Order `json:"orders"`
	Meta   MetaData       `json:"meta"`
}

type MetaData struct {
	Page        int   `json:"page"`
	Limit       int   `json:"limit"`
	TotalOrders int64 `json:"total_orders"`
	TotalPages  int64 `json:"total_pages"`
	HasMore     bool  `json:"has_more"`
}

type ServiceError struct {
	StatusCode int
	Message    string
}

func (e *ServiceError) Error() string {
	return e.Message
}

type OrderService struct {
	orderRepo     repositories.OrderRepository
	kafkaProducer kafka.ProducerAPI
	checkoutTopic string
	snsClient     *aws_pkg.SNSClient
	snsTopicArn   string
}

func NewOrderService(orderRepo repositories.OrderRepository, kafkaProducer kafka.ProducerAPI, checkoutTopic string, snsClient aws_pkg.SNSPublisher, snsTopicArn string) *OrderService {
	return &OrderService{
		orderRepo:     orderRepo,
		kafkaProducer: kafkaProducer,
		checkoutTopic: checkoutTopic,
		snsClient:     snsClient,
		snsTopicArn:   snsTopicArn,
	}
}

// CreateOrder processes order creation
func (s *OrderService) CreateOrder(ctx context.Context, userID string, req *CreateOrderRequest) *ServiceError {
	if len(req.Items) == 0 {
		return &ServiceError{
			StatusCode: 400,
			Message:    "At least one item is required",
		}
	}

	// Build event items
	eventItems := make([]models.CheckoutItem, 0, len(req.Items))
	for _, item := range req.Items {
		eventItems = append(eventItems, models.CheckoutItem{
			ProductID: item.ProductID.String(),
			Quantity:  item.Quantity,
		})
	}

	// Create checkout event
	checkoutEvent := models.CheckoutEvent{
		UserID:    userID,
		Items:     eventItems,
		Timestamp: time.Now(),
	}

	eventBytes, err := json.Marshal(checkoutEvent)
	if err != nil {
		log.Printf("[OrderService] Failed to marshal checkout event: %v", err)
		return &ServiceError{
			StatusCode: 500,
			Message:    "Failed to process order",
		}
	}

	// Publish to Kafka
	if err := s.kafkaProducer.Publish(s.checkoutTopic, eventBytes); err != nil {
		log.Printf("[OrderService] Failed to publish to Kafka: %v", err)
		return &ServiceError{
			StatusCode: 500,
			Message:    "Failed to publish checkout event",
		}
	}

	// Optionally publish to SNS (best-effort)
	if s.snsClient != nil && s.snsTopicArn != "" {
		if err := s.snsClient.Publish(ctx, s.snsTopicArn, eventBytes); err != nil {
			log.Printf("[OrderService] SNS publish failed: %v", err)
			// don't fail the request if SNS fails; it's best-effort for now
		} else {
			log.Printf("[OrderService] SNS published to %s", s.snsTopicArn)
		}
	}

	log.Printf("[OrderService] Order creation initiated for user: %s", userID)
	return nil
}

// GetUserOrders retrieves paginated orders for a specific user
func (s *OrderService) GetUserOrders(ctx context.Context, userID string, page, limit int) (*OrderResponse, *ServiceError) {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, &ServiceError{
			StatusCode: 400,
			Message:    "Invalid user ID format",
		}
	}

	orders, total, err := s.orderRepo.FindByUserID(ctx, userUUID, page, limit)
	if err != nil {
		log.Printf("[OrderService] Failed to fetch orders for user %s: %v", userID, err)
		return nil, &ServiceError{
			StatusCode: 500,
			Message:    "Failed to fetch orders",
		}
	}

	return &OrderResponse{
		Orders: orders,
		Meta: MetaData{
			Page:        page,
			Limit:       limit,
			TotalOrders: total,
			TotalPages:  calculateTotalPages(total, limit),
			HasMore:     total > int64(page*limit),
		},
	}, nil
}

// GetAllOrders retrieves paginated orders for all users (admin only)
func (s *OrderService) GetAllOrders(ctx context.Context, adminID string, page, limit int) (*OrderResponse, *ServiceError) {
	log.Printf("[OrderService] Admin %s accessing all orders", adminID)

	orders, total, err := s.orderRepo.FindAll(ctx, page, limit)
	if err != nil {
		log.Printf("[OrderService] Failed to fetch all orders: %v", err)
		return nil, &ServiceError{
			StatusCode: 500,
			Message:    "Failed to fetch orders",
		}
	}

	return &OrderResponse{
		Orders: orders,
		Meta: MetaData{
			Page:        page,
			Limit:       limit,
			TotalOrders: total,
			TotalPages:  calculateTotalPages(total, limit),
			HasMore:     total > int64(page*limit),
		},
	}, nil
}

// GetOrderByID retrieves a specific order for a user
func (s *OrderService) GetOrderByID(ctx context.Context, userID string, order_id uuid.UUID) (*models.Order, *ServiceError) {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, &ServiceError{
			StatusCode: 400,
			Message:    "Invalid user ID format",
		}
	}

	order, err := s.orderRepo.FindByIDAndUserID(ctx, order_id, userUUID)
	if err != nil {
		if err.Error() == "record not found" {
			return nil, &ServiceError{
				StatusCode: 404,
				Message:    "Order not found",
			}
		}
		log.Printf("[OrderService] Failed to fetch order %s for user %s: %v", order_id, userID, err)
		return nil, &ServiceError{
			StatusCode: 500,
			Message:    "Failed to fetch order",
		}
	}

	return order, nil
}

func calculateTotalPages(total int64, limit int) int64 {
	if limit == 0 {
		return 0
	}
	return (total + int64(limit) - 1) / int64(limit)
}
