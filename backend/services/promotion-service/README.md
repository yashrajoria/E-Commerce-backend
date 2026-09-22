# Promotion Service

The promotion service owns coupon definitions, validation, usage limits, and deactivation. It is a Go/Gin service on port `8090` backed by PostgreSQL and an order-event consumer.

## Responsibilities

- Validate a coupon against cart total, dates, status, and usage limits.
- Return a coupon for authenticated checkout flows.
- Create, list, and deactivate coupons for administrators.
- Atomically consume coupon usage when an order event is finalized; concurrent attempts cannot exceed `usage_limit`.
- Publish promotion-related notification events when configured.

## Architecture

```mermaid
flowchart LR
  Client[Client / BFF] --> Gateway[API Gateway :8080]
  Gateway --> Promo[Promotion Service :8090\nGo / Gin]
  Promo --> PG[(PostgreSQL\ncoupons)]
  OrderEvents[SNS order events] --> Queue[SQS promotion-order-queue]
  Queue --> Promo
  Promo --> Notify[(SNS promotion-events / notifications)]
  Admin[Admin] -->|JWT + admin role| Gateway
```

## Coupon flow

```mermaid
sequenceDiagram
  participant Client
  participant Gateway
  participant Promo
  participant DB as PostgreSQL
  participant OrderEvents as SQS order events

  Client->>Gateway: POST /coupons/validate
  Gateway->>Promo: code + cart_total
  Promo->>DB: Load active coupon
  Promo-->>Client: Discount calculation
  OrderEvents->>Promo: Order-created event
  Promo->>DB: Atomic usage-limit update
```

## HTTP surface

| Route | Purpose | Access |
| --- | --- | --- |
| `POST /coupons/validate` | Validate a code for a cart | Public, rate-limited at edge |
| `GET /coupons/:code` | Read coupon details | Authenticated |
| `POST /coupons` | Create coupon | Admin |
| `GET /coupons` | List coupons | Admin |
| `DELETE /coupons/:code` | Deactivate coupon | Admin |

## Persistence and consistency

The `coupons` table is owned by this service. Usage increments must be atomic so concurrent orders cannot exceed a coupon's limit. Order-event consumption is retry-safe and should acknowledge an event only after the database transition completes.

## Configuration

Configure Postgres, `ALLOW_AUTO_MIGRATE`, SNS/SQS topic and queue names, AWS/LocalStack settings, and service authentication values. Apply the coupon migration before startup.
