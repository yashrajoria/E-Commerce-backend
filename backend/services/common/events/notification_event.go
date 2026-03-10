package events

type NotificationEvent struct {
	EventType string                 `json:"event_type"`
	UserID    string                 `json:"user_id"`
	Recipient string                 `json:"recipient"`
	Data      map[string]interface{} `json:"data"`
}

// NotificationItem represents a line item in order notification emails.
type NotificationItem struct {
	ProductName string  `json:"product_name"`
	Quantity    int     `json:"quantity"`
	Price       float64 `json:"price"`
}

func NewOrderCreatedEvent(userID, email, phone, name, orderID string, total float64, items []NotificationItem) NotificationEvent {
	return NotificationEvent{
		EventType: "order_created",
		UserID:    userID,
		Recipient: email,
		Data: map[string]interface{}{
			"name":     name,
			"email":    email,
			"phone":    phone,
			"order_id": orderID,
			"total":    total,
			"items":    items,
		},
	}
}

// NewOrderConfirmedEvent creates a notification event after payment succeeds,
// including the purchased items so the email contains product details.
func NewOrderConfirmedEvent(userID, email, name, orderID string, total float64, items []NotificationItem) NotificationEvent {
	return NotificationEvent{
		EventType: "order_confirmed",
		UserID:    userID,
		Recipient: email,
		Data: map[string]interface{}{
			"name":     name,
			"email":    email,
			"order_id": orderID,
			"total":    total,
			"items":    items,
		},
	}
}

func NewOrderShippedEvent(userID, email, name, orderID, trackingCode string) NotificationEvent {
	return NotificationEvent{
		EventType: "order_shipped",
		UserID:    userID,
		Recipient: email,
		Data: map[string]interface{}{
			"name":          name,
			"email":         email,
			"order_id":      orderID,
			"tracking_code": trackingCode,
		},
	}
}

func NewOrderDeliveredEvent(userID, email, name, orderID string) NotificationEvent {
	return NotificationEvent{
		EventType: "order_delivered",
		UserID:    userID,
		Recipient: email,
		Data: map[string]interface{}{
			"name":     name,
			"email":    email,
			"order_id": orderID,
		},
	}
}

func NewPaymentFailedEvent(userID, email, phone, name, orderID string, amount float64) NotificationEvent {
	return NotificationEvent{
		EventType: "payment_failed",
		UserID:    userID,
		Recipient: email,
		Data: map[string]interface{}{
			"name":     name,
			"email":    email,
			"phone":    phone,
			"order_id": orderID,
			"amount":   amount,
		},
	}
}

func NewCouponAppliedEvent(userID, email, name, couponCode string, discount float64) NotificationEvent {
	return NotificationEvent{
		EventType: "coupon_applied",
		UserID:    userID,
		Recipient: email,
		Data: map[string]interface{}{
			"name":     name,
			"email":    email,
			"code":     couponCode,
			"discount": discount,
		},
	}
}
