# ADR 002: Transactional Outbox Pattern for Asynchronous Events

## Status
Accepted

## Context
When an order is created or a payment status changes, downstream services (such as inventory, notifications, and analytics) must be reliably notified via AWS SQS/SNS. Writing to the database and publishing to a message broker in separate steps introduces a **dual-write hazard**:
- If the database commit succeeds but message publishing fails (e.g. network partition, process crash), the message is lost forever.
- If publishing succeeds but the database transaction rolls back, downstream services act on phantom data.
- Heavy distributed solutions like Kafka + Debezium CDC introduce significant operational overhead and infrastructure requirements.

## Decision
Implement the **Transactional Outbox Pattern** in `order-service` and `payment-service`:
1. Events are written to an `outbox_events` (or `payment_outbox_events`) table within the **same local database transaction** as the business entity changes (`orders` or `payments`).
2. An asynchronous worker (`OutboxPublisher`) polls claimable pending events using optimistic concurrency / row leasing (`UPDATE ... WHERE status = 'pending' ... LIMIT n`).
3. Events are published to their designated AWS SQS queue or SNS topic with exponential backoff.
4. Upon successful delivery acknowledgement from AWS SDK, the event status transitions to `published` with `published_at` recorded.
5. Filtered partial indexes (`idx_outbox_events_claimable`) keep polling queries sub-millisecond.

## Consequences
- **Positive**: Guaranteed at-least-once message delivery; eliminates dual-write race conditions; zero additional external infrastructure needed.
- **Negative**: Downstream consumers must be idempotent (accommodating at-least-once delivery); slight delivery latency (polling interval ~500ms).
