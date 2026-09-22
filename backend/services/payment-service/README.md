# Payment Service

The payment service owns payment records, Stripe Checkout session creation, webhook verification, and payment event publication. It is a Go/Gin service on port `8087` with a background SQS consumer.

## Responsibilities

- Create Stripe Checkout sessions for pending orders.
- Persist payment state and checkout URLs.
- Verify payment status for authenticated clients.
- Validate Stripe webhook signatures and process terminal transitions once.
- Publish payment success/failure events for order and notification consumers.
- Retry SQS payment requests without creating duplicate payment sessions.
- Deduplicate payment requests by stable `event_id`; legacy messages fall back to `idempotency_key`.

## Architecture

```mermaid
flowchart LR
  Order[Order Service :8083] --> RequestQ[SQS payment-request-queue]
  RequestQ --> Payment[Payment Service :8087\nGo / Gin]
  Payment --> Stripe[(Stripe API)]
  Stripe -->|signed webhook| Gateway[API Gateway :8080]
  Gateway --> Payment
  Payment --> PG[(PostgreSQL\npayments\nstripe_processed_events)]
  Payment --> Events[(SNS payment-events)]
  Events --> OrderQ[SQS payment-events-queue]
  Events --> NotifyQ[SQS notification-queue]
```

## Payment flow

```mermaid
sequenceDiagram
  participant Order
  participant RequestQ as Payment Request Queue
  participant Payment
  participant Stripe
  participant DB as PostgreSQL
  participant Events as SNS payment-events

  Order->>RequestQ: Enqueue payment request
  RequestQ->>Payment: Consume idempotency key
  Payment->>DB: Create or load payment row
  Payment->>Stripe: Create Checkout Session
  Stripe-->>Payment: checkout_url
  Payment->>DB: Store session and pending state
  Stripe->>Payment: Signed payment webhook
  Payment->>DB: Atomically transition payment to succeeded
  Payment->>Events: Publish payment.succeeded once
```

## HTTP surface

| Route | Purpose | Access |
| --- | --- | --- |
| `GET /payment/status/by-order/:order_id` | Read payment state and checkout URL | Authenticated |
| `POST /payment/create-checkout` | Create a checkout session directly | Authenticated |
| `POST /payment/verify-payment` | Verify a payment status | Authenticated |
| `POST /stripe/webhook` | Receive Stripe events | Stripe signature |
| `GET /health`, `/health/live`, `/health/ready` | Liveness/readiness | Public |

## Persistence and webhook safety

`payments` is the service's payment state; `stripe_processed_events` records Stripe event IDs for audit and deduplication. The payment row transition is guarded so concurrent or repeated webhook deliveries cannot publish fulfillment events twice. The webhook returns an acknowledgement only after signature and processing rules have been applied.

## Configuration

Set Stripe secret/signing keys, frontend URL settings, Postgres, SQS/SNS, AWS/LocalStack, and `INTERNAL_SERVICE_TOKEN` where required. Never expose Stripe secrets to clients or place them in repository configuration.
