package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"order-service/models"
	paymentservices "order-service/payment/services"
	repositories "order-service/repository"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yashrajoria/common/messaging"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type ContextKey string

const IdempotencyKeyContextKey ContextKey = "idempotency_key"

type CreateOrderRequest struct {
	Items []struct {
		ProductID uuid.UUID `json:"product_id" binding:"required"`
		Quantity  int       `json:"quantity" binding:"required,min=1"`
	} `json:"items" binding:"required,dive"`
	StoreID *uuid.UUID `json:"store_id,omitempty"`
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
// needs to release reserved stock on cancellation and restock confirmed stock
// when a paid order is cancelled. Defined here (rather than depending on the
// concrete *InventoryClient) so tests can supply a fake.
type InventoryReleaser interface {
	ReleaseStock(ctx context.Context, orderID string, items []ReserveItem) error
	RestockStock(ctx context.Context, orderID string, items []ReserveItem) error
}

// cancellableStatuses are the order statuses an admin may cancel from.
// Anything else (already cancelled, completed, payment_failed, etc.) is a
// terminal or non-cancellable state.
var cancellableStatuses = map[string]bool{
	"pending_payment": true,
	"paid":            true,
}

// refundableStatuses are the order statuses an order may be refunded from.
var refundableStatuses = map[string]bool{
	"paid":      true,
	"fulfilled": true,
	"completed": true,
}

type RefundOrderRequest struct {
	Reason string `json:"reason,omitempty"`
	Amount *int   `json:"amount,omitempty"`
}

type OrderService struct {
	orderRepo            repositories.OrderRepository
	snsClient            messaging.Publisher
	snsTopicArn          string
	notificationTopicArn string
	inventoryClient      InventoryReleaser
	refunder             PaymentRefunder
}

// PaymentRefunder refunds the captured payment of a paid order (idempotent).
type PaymentRefunder interface {
	RefundOrder(ctx context.Context, orderID uuid.UUID) error
}

// DetailedPaymentRefunder provides full/partial refund with details.
type DetailedPaymentRefunder interface {
	RefundPayment(ctx context.Context, orderID uuid.UUID, amount *int, reason string, idempotencyKey string) (*paymentservices.RefundDetails, error)
}

// SetRefunder enables Stripe refunds when a paid order is cancelled.
func (s *OrderService) SetRefunder(r PaymentRefunder) { s.refunder = r }

// NewOrderServiceSQS creates an OrderService that uses SNS/SQS instead of Kafka
func NewOrderServiceSQS(orderRepo repositories.OrderRepository, snsClient messaging.Publisher, snsTopicArn, notificationTopicArn string, inventoryClient InventoryReleaser) *OrderService {
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
		Timestamp: time.Now().UTC(),
	}

	if req.StoreID != nil {
		checkoutEvent.StoreID = req.StoreID.String()
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

// GetStoreOrders retrieves paginated orders for a specific store (tenant isolation)
func (s *OrderService) GetStoreOrders(ctx context.Context, storeID uuid.UUID, page, limit int) (*OrderResponse, *ServiceError) {
	orders, total, err := s.orderRepo.FindByStoreID(ctx, storeID, page, limit)
	if err != nil {
		zap.L().Error("failed to fetch store orders", zap.Error(err), zap.String("store_id", storeID.String()))
		return nil, &ServiceError{
			StatusCode: 500,
			Message:    "Failed to fetch store orders",
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

// CancelOrder transitions an order to "cancelled" (admin action) and restores
// its inventory. Only pending_payment/paid orders can be cancelled; anything
// else (already cancelled, completed, payment_failed) is rejected with 409 so
// the caller doesn't silently no-op on a terminal order.
//
// Inventory handling differs by prior status:
//   - pending_payment: reservation still exists → ReleaseStock (available += qty, reserved -= qty)
//   - paid: ConfirmStock already removed the reservation while keeping available
//     deducted → RestockStock (available += qty) so stock isn't permanently depleted.
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

	wasPaid := order.Status == "paid"
	// Refund before changing state: a failed refund leaves the order paid and cancellable
	// again, and the idempotent refund makes a retry after a later failure safe.
	if wasPaid && s.refunder != nil {
		if err := s.refunder.RefundOrder(ctx, orderID); err != nil {
			zap.L().Error("failed to refund cancelled paid order", zap.String("order_id", orderID.String()), zap.Error(err))
			return nil, &ServiceError{StatusCode: 502, Message: "Failed to refund payment; order not cancelled"}
		}
	}
	now := time.Now().UTC()
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
		// Best-effort: an inventory failure shouldn't undo the cancellation (mirrors
		// the existing release-on-payment-failure behavior in sqs_payment_consumer.go).
		if wasPaid {
			if restErr := s.inventoryClient.RestockStock(ctx, orderID.String(), items); restErr != nil {
				zap.L().Error("failed to restock inventory for cancelled paid order",
					zap.String("order_id", orderID.String()), zap.Error(restErr))
			}
		} else if relErr := s.inventoryClient.ReleaseStock(ctx, orderID.String(), items); relErr != nil {
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

// RefundOrder processes a refund for an order in eligible status (e.g., PAID or FULFILLED).
// It validates order state, prevents double-refunding, calls Stripe's refund API via the
// payment refunder, updates order status to "refunded", restocks inventory, and emits
// a payment_refunded event.
func (s *OrderService) RefundOrder(ctx context.Context, orderID uuid.UUID, callerID, callerRole string, req *RefundOrderRequest) (*models.Order, *ServiceError) {
	order, err := s.orderRepo.FindByID(ctx, orderID)
	if err != nil {
		if err.Error() == "record not found" || errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, &ServiceError{StatusCode: 404, Message: "Order not found"}
		}
		zap.L().Error("failed to fetch order for refund", zap.String("order_id", orderID.String()), zap.Error(err))
		return nil, &ServiceError{StatusCode: 500, Message: "Failed to fetch order"}
	}

	// Authorization check: admin or order owner
	if callerRole != "admin" && order.UserID.String() != callerID {
		return nil, &ServiceError{StatusCode: 403, Message: "Access denied"}
	}

	currentStatus := strings.ToLower(strings.TrimSpace(order.Status))
	if currentStatus == "refunded" {
		return nil, &ServiceError{
			StatusCode: 409,
			Message:    "order is already refunded",
		}
	}

	if !refundableStatuses[currentStatus] {
		return nil, &ServiceError{
			StatusCode: 409,
			Message:    fmt.Sprintf("order cannot be refunded from status %q", order.Status),
		}
	}

	if req.Amount != nil {
		if *req.Amount <= 0 {
			return nil, &ServiceError{StatusCode: 400, Message: "Refund amount must be greater than zero"}
		}
		if *req.Amount > order.Amount {
			return nil, &ServiceError{StatusCode: 400, Message: "Refund amount exceeds order total"}
		}
	}

	if s.refunder == nil {
		return nil, &ServiceError{StatusCode: 500, Message: "Payment refunder not configured"}
	}

	idemKey := ""
	if v, ok := ctx.Value(IdempotencyKeyContextKey).(string); ok && v != "" {
		idemKey = v
	}

	var refundDetails *paymentservices.RefundDetails
	if detailed, ok := s.refunder.(DetailedPaymentRefunder); ok {
		var rErr error
		refundDetails, rErr = detailed.RefundPayment(ctx, orderID, req.Amount, req.Reason, idemKey)
		if rErr != nil {
			if errors.Is(rErr, paymentservices.ErrPaymentNotFound) {
				return nil, &ServiceError{StatusCode: 404, Message: "Payment record not found for order"}
			}
			if errors.Is(rErr, paymentservices.ErrPaymentAlreadyRefunded) {
				return nil, &ServiceError{StatusCode: 409, Message: "Payment is already refunded"}
			}
			if errors.Is(rErr, paymentservices.ErrPaymentNotSucceeded) {
				return nil, &ServiceError{StatusCode: 409, Message: rErr.Error()}
			}
			if errors.Is(rErr, paymentservices.ErrMissingStripePaymentID) {
				return nil, &ServiceError{StatusCode: 400, Message: rErr.Error()}
			}
			zap.L().Error("failed to refund payment via Stripe", zap.String("order_id", orderID.String()), zap.Error(rErr))
			return nil, &ServiceError{StatusCode: 502, Message: fmt.Sprintf("Failed to refund payment: %v", rErr)}
		}
	} else {
		if err := s.refunder.RefundOrder(ctx, orderID); err != nil {
			if errors.Is(err, paymentservices.ErrPaymentNotFound) {
				return nil, &ServiceError{StatusCode: 404, Message: "Payment record not found for order"}
			}
			zap.L().Error("failed to refund payment via refunder", zap.String("order_id", orderID.String()), zap.Error(err))
			return nil, &ServiceError{StatusCode: 502, Message: fmt.Sprintf("Failed to refund payment: %v", err)}
		}
	}

	now := time.Now().UTC()
	extra := map[string]interface{}{
		"updated_at": now,
	}
	if err := s.orderRepo.UpdateOrderStatus(ctx, orderID, order.Status, "refunded", extra); err != nil {
		if errors.Is(err, repositories.ErrStatusConflict) {
			return nil, &ServiceError{StatusCode: 409, Message: "order status changed concurrently, please retry"}
		}
		zap.L().Error("failed to update order status to refunded", zap.String("order_id", orderID.String()), zap.Error(err))
		return nil, &ServiceError{StatusCode: 500, Message: "Failed to update order status to refunded"}
	}

	order.Status = "refunded"
	order.UpdatedAt = now

	// Restock inventory for refunded paid/fulfilled orders
	if s.inventoryClient != nil && len(order.OrderItems) > 0 {
		items := make([]ReserveItem, 0, len(order.OrderItems))
		for _, item := range order.OrderItems {
			items = append(items, ReserveItem{ProductID: item.ProductID.String(), Quantity: item.Quantity})
		}
		if restErr := s.inventoryClient.RestockStock(ctx, orderID.String(), items); restErr != nil {
			zap.L().Error("failed to restock inventory for refunded order",
				zap.String("order_id", orderID.String()), zap.Error(restErr))
		}
	}

	// Publish refunded event
	refundedAmount := order.Amount
	if req.Amount != nil && *req.Amount > 0 {
		refundedAmount = *req.Amount
	}
	paymentEvt := models.PaymentEvent{
		Type:      "payment_refunded",
		OrderID:   orderID.String(),
		UserID:    order.UserID.String(),
		Amount:    refundedAmount,
		Currency:  order.Currency,
		Timestamp: now,
	}
	if refundDetails != nil {
		paymentEvt.PaymentID = refundDetails.PaymentID
		paymentEvt.CorrelationID = refundDetails.CorrelationID
	}
	if eventBytes, err := json.Marshal(paymentEvt); err == nil {
		if s.snsClient != nil && s.snsTopicArn != "" {
			if err := s.snsClient.Publish(ctx, s.snsTopicArn, eventBytes); err != nil {
				zap.L().Error("failed to publish payment_refunded event to SNS", zap.Error(err))
			}
		}
	}

	zap.L().Info("[AUDIT] order refunded",
		zap.String("order_id", orderID.String()),
		zap.String("caller_id", callerID),
		zap.String("reason", req.Reason),
		zap.Int("amount", refundedAmount),
	)

	return order, nil
}

func calculateTotalPages(total int64, limit int) int64 {
	if limit == 0 {
		return 0
	}
	return (total + int64(limit) - 1) / int64(limit)
}
