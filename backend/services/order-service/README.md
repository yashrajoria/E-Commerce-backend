# Order Service

The order service owns order state, order items, checkout consumption, stock orchestration, and payment dispatch. It is a Go/Gin service on port `8083` with PostgreSQL persistence and SQS consumers.

## Responsibilities

- Consume checkout requests and create idempotent orders.
- Validate products and coupons through internal service clients.
- Reserve inventory before finalizing the order's pending state.
- Dispatch payment requests and consume payment events.
- Confirm or cancel orders and release inventory when payment fails.
- Expose customer order history and admin order/revenue views.

## Architecture

```mermaid
flowchart LR
  Client[Client] --> Gateway[API Gateway :8080]
  Gateway --> Order[Order Service :8083\nGo / Gin]
  SNS[(SNS order-events)] --> CheckoutQ[SQS order-processing-queue]
  CheckoutQ --> Order
  Order --> PG[(PostgreSQL\norders\norder_items)]
  Order --> Inventory[Inventory Service :8084]
  Order --> Product[Product Service :8082\ninternal API]
  Order --> Promotion[Promotion Service :8090\ninternal/API]
  Order --> PaymentQ[SQS payment-request-queue]
  PaymentEvents[SQS payment-events-queue] --> Order
```

## Checkout and payment flow

```mermaid
sequenceDiagram
  participant Cart
  participant SNS
  participant CheckoutQ as Order Queue
  participant Order
  participant Inventory
  participant DB as PostgreSQL
  participant PaymentQ as Payment Queue
  participant Payment

  Cart->>SNS: checkout.requested
  SNS->>CheckoutQ: Deliver message
  CheckoutQ->>Order: Consume with idempotency key
  Order->>Product: Validate product snapshot
  Order->>Inventory: Reserve stock
  Order->>DB: Insert pending order and items
  Order->>PaymentQ: Enqueue payment request
  PaymentQ->>Payment: Create checkout session
  Payment-->>Order: payment.succeeded via payment-events
  Order->>DB: Mark paid and confirm inventory
```

## HTTP surface

| Route | Purpose | Access |
| --- | --- | --- |
| `GET /orders/` | List the current user's orders | Authenticated |
| `GET /orders/:id` | Read one owned order | Authenticated |
| `GET /orders/admin/` | List all orders | Admin |
| `GET /orders/admin/stats` | Read revenue/order statistics | Admin |

## Persistence and idempotency

PostgreSQL tables `orders` and `order_items` belong exclusively to this service. Checkout consumers deduplicate on the order idempotency key. Inventory operations are tokenized; payment-event handling checks order state before applying a terminal transition. SQS consumers must be safe to retry because acknowledgement happens only after successful processing.

## Configuration

Configure Postgres, AWS SQS/SNS, internal service URLs, `INTERNAL_SERVICE_TOKEN`, and product/inventory/promotion client settings. Apply migrations from `backend/migrations` before starting in production.
