# ADR 003: Stripe Hosted Checkout & Webhook Idempotency

## Status
Accepted

## Context
E-commerce payment processing requires PCI-DSS compliance, fraud protection, and support for multiple payment methods (cards, Apple Pay, Google Pay). Directly capturing and handling raw credit card credentials on the platform increases security obligations, audit complexity, and vulnerability surface.

## Decision
Use **Stripe Hosted Checkout Sessions** and **Webhook-driven Asynchronous Fulfillment**:
1. When checkout is initiated, `payment-service` generates a Stripe Checkout Session via Stripe API with embedded metadata (`order_id`, `user_id`).
2. The user is redirected to Stripe's secure hosted payment page, removing sensitive payment details from ShopSwift's network perimeter.
3. Stripe emits webhook events (`checkout.session.completed`, `payment_intent.succeeded`, `payment_intent.payment_failed`) to the API Gateway edge `/stripe/webhook`.
4. `payment-service` verifies the HMAC signature using `STRIPE_WEBHOOK_SECRET` and performs an atomic state transition:
   - Gated by terminal status checks to avoid reprocessing redeliveries.
   - Idempotency recorded in `stripe_processed_events` table.
   - Enqueues `payment_succeeded` outbox event inside the database transaction.

## Consequences
- **Positive**: Minimum PCI DSS scope (SAQ-A); high conversion rate with localized Stripe payment UI; resilient to network drops between customer browser and server.
- **Negative**: Customer leaves the application domain temporarily; order confirmation depends on webhook delivery reliability.
