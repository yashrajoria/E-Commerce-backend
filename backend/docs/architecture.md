# Backend Architecture

High-level architecture of the ShopSwift e-commerce backend and how components interact.

Related: [data-and-messaging.md](./data-and-messaging.md) · [best-practices-and-gaps.md](./best-practices-and-gaps.md) · [api/README.md](./api/README.md)

## Overview diagram

```mermaid
flowchart LR
  subgraph Users
    U[Frontend_Client]
  end

  subgraph Edge
    API[API_Gateway_8080]
  end

  subgraph BFF_Service
    BFF[BFF_8088]
  end

  subgraph Services
    AUTH[Auth_8081]
    USER[User_8085]
    PROD[Product_8082]
    CART[Cart_8086]
    ORDER[Order_8083]
    PAYMENT[Payment_8087]
    INVENT[Inventory_8084]
    PROMO[Promotion_8090]
    SHIP[Shipping_8091]
    NOTIF[Notification_8092]
    AGENT[Agent_8089]
  end

  subgraph Infrastructure
    PG[(Postgres)]
    REDIS[(Redis)]
    S3[(S3)]
    SNS_SQS[SNS_SQS]
    DDB[(DynamoDB)]
    CW[CloudWatch]
    STRIPE[(Stripe)]
  end

  U -->|HTTP_cookies| API
  U -->|optional| BFF
  API -->|proxy| BFF
  API -->|proxy| AUTH
  API -->|proxy| USER
  API -->|proxy| PROD
  API -->|proxy| CART
  API -->|proxy| ORDER
  API -->|proxy| PAYMENT
  API -->|proxy| INVENT
  API -->|proxy| PROMO
  API -->|proxy| SHIP
  API -->|proxy| NOTIF
  API -->|proxy| AGENT

  BFF -->|HTTP_via_gateway| API
  BFF -->|idempotency| REDIS

  CART --> REDIS
  ORDER --> PG
  AUTH --> PG
  USER --> PG
  PROMO --> PG
  NOTIF --> PG
  PAYMENT --> PG

  PROD --> S3
  PROD --> DDB
  PROD --> REDIS
  INVENT --> DDB

  CART -->|SNS_order_events| SNS_SQS
  ORDER --> SNS_SQS
  PAYMENT --> SNS_SQS
  PAYMENT --> STRIPE
  NOTIF -->|SQS_consume| SNS_SQS
  ORDER -->|reserve| INVENT

  AGENT -->|BFF_direct| BFF
```

Source also maintained in [architecture.mmd](./architecture.mmd).

## Narrative

- Clients talk to the **API Gateway** (`:8080`). The gateway proxies many services **directly** and also fronts the **BFF** (`/bff`).
- The **BFF** (`:8088`) aggregates home/profile/checkout. Checkout uses a Redis **SetNX** lock keyed by `Idempotency-Key`, then drives an **async** cart → SNS/SQS → order → payment flow and polls for a Stripe `checkout_url`.
- **Postgres:** auth (identity + refresh tokens), user (profile + addresses), order, payment, promotion (coupons), notification logs.
- **DynamoDB:** product catalog, categories, inventory (primary — not optional).
- **Redis:** cart state, BFF checkout locks, gateway rate limiting, product cache.
- **Shipping** computes rates only (zone/static JSON); it does **not** write Postgres at runtime.
- **Cart** is Redis-only (no Postgres writes).
- **Notification** consumes `notification-queue` (SNS `notification-events`) and sends email / logs.
- **Agent** (Python) calls BFF directly to avoid gateway circularity; needs an LLM endpoint.
- **LocalStack** emulates S3/SNS/SQS/DynamoDB locally — required for local AWS-dependent services.

## Sequence diagrams

### Checkout (async SNS/SQS)

```mermaid
sequenceDiagram
  participant Client
  participant Gateway
  participant BFF
  participant Redis
  participant Cart
  participant SNS
  participant Order
  participant SQS
  participant Payment
  participant Stripe

  Client->>Gateway: POST /bff/checkout Idempotency-Key
  Gateway->>BFF: forward
  BFF->>Redis: SetNX checkout lock pending
  BFF->>Cart: POST /cart/checkout
  Cart->>SNS: publish checkout.requested order-events
  Cart-->>BFF: order_id
  SNS->>SQS: order-processing-queue
  Order->>Order: create order reserve inventory
  Order->>SQS: payment-request-queue
  Payment->>Stripe: create Checkout Session
  Payment->>SNS: payment-events
  BFF->>Payment: poll status for checkout_url
  BFF->>Redis: store order_id on lock
  BFF-->>Client: checkout_url
```

### Payment webhook confirmation

```mermaid
sequenceDiagram
  participant Stripe
  participant Gateway
  participant Payment
  participant SNS
  participant OrderSQS as order_payment_events_queue
  participant Order
  participant NotifQ as notification_queue
  participant Notif as notification_service

  Stripe->>Gateway: POST /stripe/webhook
  Gateway->>Payment: forward
  Payment->>Payment: dedupe by Stripe event.id
  Payment->>SNS: payment-events paid
  SNS->>OrderSQS: deliver
  Order->>Order: mark paid confirm inventory
  Payment->>SNS: notification-events
  SNS->>NotifQ: deliver
  Notif->>Notif: email and log
  Payment-->>Gateway: 200
```

### Idempotency keys

| Hop | Mechanism |
|-----|-----------|
| Client → BFF | Required `Idempotency-Key` header + Redis SetNX |
| Cart → Order | Order `idempotency_key` unique; SQS consumer dedups |
| Payment request | Payment `idempotency_key` unique |
| Stripe webhook | Stored Stripe `event.id` (processed events) |
