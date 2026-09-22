package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"order-service/models"
	repositories "order-service/repository"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"

	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ContextKey string

const IdempotencyKeyContextKey ContextKey = "idempotency_key"

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

// InventoryReleaser is the subset of InventoryClient's behavior OrderService
// needs to release reserved stock on cancellation. Defined here (rather than
// depending on the concrete *InventoryClient) so tests can supply a fake.
type InventoryReleaser interface {
	ReleaseStock(ctx context.Context, orderID string, items []ReserveItem) error
}

// cancellableStatuses are the order statuses an admin may cancel from.
// Anything else (already cancelled, completed, payment_failed, etc.) is a
// terminal or non-cancellable state.
var cancellableStatuses = map[string]bool{
	"pending_payment": true,
	"paid":            true,
}

type OrderService struct {
	orderRepo            repositories.OrderRepository
	snsClient            aws_pkg.SNSPublisher
	snsTopicArn          string
	notificationTopicArn string
	inventoryClient      InventoryReleaser
}

// NewOrderServiceSQS creates an OrderService that uses SNS/SQS instead of Kafka
func NewOrderServiceSQS(orderRepo repositories.OrderRepository, snsClient aws_pkg.SNSPublisher, snsTopicArn, notificationTopicArn string, inventoryClient InventoryReleaser) *OrderService {
	return &OrderService{
		orderRepo:            orderRepo,
		snsClient:            snsClient,
		snsTopicArn:          snsTopicArn,
		notificationTopicArn: notificationTopicArn,
		inventoryClient:      inventoryClient,
	}
}

// CreateOrder processes order creation via SNS
func (s *OrderService) CreateOrder(ctx context.Context, userID, email string, req *CreateOrderRequest) *ServiceError {
	if len(req.Items) == 0 {
		return &ServiceError{
			StatusCode: 400,
			Message:    "At least one item is required",
		}
	}

	orderID := uuid.New().String()

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
		Email:     email,
		OrderID:   orderID,
		Items:     eventItems,
		Timestamp: time.Now(),
	}

	if idemKey, ok := ctx.Value(IdempotencyKeyContextKey).(string); ok && idemKey != "" {
		checkoutEvent.IdempotencyKey = idemKey
	}

	eventBytes, err := json.Marshal(checkoutEvent)
	if err != nil {
		zap.L().Error("failed to marshal checkout event", zap.Error(err))
		return &ServiceError{
			StatusCode: 500,
			Message:    "Failed to process order",
		}
	}

	// Publish to SNS (which fans out to SQS queues)
	if s.snsClient != nil && s.snsTopicArn != "" {
		if err := s.snsClient.Publish(ctx, s.snsTopicArn, eventBytes); err != nil {
			zap.L().Error("sns publish failed", zap.Error(err))
			return &ServiceError{
				StatusCode: 500,
				Message:    "Failed to publish checkout event",
			}
		}
		zap.L().Info("sns published", zap.String("topic", s.snsTopicArn))
		// NOTE: order_created notification is now published by the checkout consumer
		// after the order is actually created in DB with correct total and items.
	} else {
		zap.L().Warn("sns client not configured, order event not published")
	}
	zap.L().Info("order creation initiated", zap.String("user", userID))
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
		zap.L().Error("failed to fetch orders for user", zap.String("user", userID), zap.Error(err))
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
	zap.L().Info("admin accessing all orders", zap.String("admin", adminID))

	orders, total, err := s.orderRepo.FindAll(ctx, page, limit)
	if err != nil {
		zap.L().Error("failed to fetch all orders", zap.Error(err))
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

func (s *OrderService) GetRevenueStats(ctx context.Context) (map[string]interface{}, *ServiceError) {
	stats, err := s.orderRepo.GetRevenueAnalytics(ctx)
	if err != nil {
		zap.L().Error("failed to fetch revenue stats", zap.Error(err))
		return nil, &ServiceError{
			StatusCode: 500,
			Message:    "Failed to fetch analytics",
		}
	}
	return stats, nil
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
		zap.L().Error("failed to fetch order for user", zap.String("order_id", order_id.String()), zap.String("user", userID), zap.Error(err))
		return nil, &ServiceError{
			StatusCode: 500,
			Message:    "Failed to fetch order",
		}
	}

	return order, nil
}

// CancelOrder transitions an order to "cancelled" (admin action) and releases
// any reserved inventory for it. Only pending_payment/paid orders can be
// cancelled; anything else (already cancelled, completed, payment_failed) is
// rejected with 409 so the caller doesn't silently no-op on a terminal order.
func (s *OrderService) CancelOrder(ctx context.Context, orderID uuid.UUID, adminID, reason string) (*models.Order, *ServiceError) {
	order, err := s.orderRepo.FindByID(ctx, orderID)
	if err != nil {
		if err.Error() == "record not found" {
			return nil, &ServiceError{StatusCode: 404, Message: "Order not found"}
		}
		zap.L().Error("failed to fetch order for cancellation", zap.String("order_id", orderID.String()), zap.Error(err))
		return nil, &ServiceError{StatusCode: 500, Message: "Failed to fetch order"}
	}

	if !cancellableStatuses[order.Status] {
		return nil, &ServiceError{
			StatusCode: 409,
			Message:    fmt.Sprintf("order cannot be cancelled from status %q", order.Status),
		}
	}

	now := time.Now()
	if err := s.orderRepo.UpdateOrderStatus(ctx, orderID, order.Status, "cancelled", map[string]interface{}{"canceled_at": now}); err != nil {
		if errors.Is(err, repositories.ErrStatusConflict) {
			return nil, &ServiceError{StatusCode: 409, Message: "order status changed concurrently, please retry"}
		}
		zap.L().Error("failed to update order status to cancelled", zap.String("order_id", orderID.String()), zap.Error(err))
		return nil, &ServiceError{StatusCode: 500, Message: "Failed to cancel order"}
	}

	if s.inventoryClient != nil && len(order.OrderItems) > 0 {
		items := make([]ReserveItem, 0, len(order.OrderItems))
		for _, item := range order.OrderItems {
			items = append(items, ReserveItem{ProductID: item.ProductID.String(), Quantity: item.Quantity})
		}
		// Best-effort: a release failure shouldn't undo the cancellation (mirrors
		// the existing release-on-payment-failure behavior in sqs_payment_consumer.go).
		if relErr := s.inventoryClient.ReleaseStock(ctx, orderID.String(), items); relErr != nil {
			zap.L().Error("failed to release inventory for cancelled order",
				zap.String("order_id", orderID.String()), zap.Error(relErr))
		}
	}

	order.Status = "cancelled"
	order.CanceledAt = &now
	zap.L().Info("[AUDIT] admin cancelled order",
		zap.String("order_id", orderID.String()),
		zap.String("admin_id", adminID),
		zap.String("reason", reason),
	)
	return order, nil
}

func calculateTotalPages(total int64, limit int) int64 {
	if limit == 0 {
		return 0
	}
	return (total + int64(limit) - 1) / int64(limit)
}
