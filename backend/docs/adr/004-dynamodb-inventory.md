# ADR 004: DynamoDB for Atomic Inventory Reservation

## Status
Accepted

## Context
High-concurrency e-commerce systems experience flash-sale spikes where multiple shoppers attempt to checkout the same constrained inventory simultaneously. Relational database row locks (`SELECT ... FOR UPDATE`) under high concurrency lead to lock contention, database connection pool exhaustion, and deadlocks.

## Decision
Use **Amazon DynamoDB** with conditional expressions and transactional writes for `inventory-service`:
1. Inventory items are tracked as document items keyed by `product_id`.
2. Reservations use DynamoDB `TransactWriteItems` or conditional `UpdateItem` with expressions (`available_quantity >= :requested_quantity`).
3. An `order_reservations` map stores the active reservations per `order_id` directly on the item, allowing idempotent release or confirmation.
4. During checkout, stock is atomically reserved before payment. If payment fails or times out, stock is compensated via `ReleaseStock`. When payment completes, `ConfirmStock` converts the reservation into a permanent inventory reduction.

## Consequences
- **Positive**: Predictable single-digit millisecond latency under arbitrary horizontal load; lock-free conditional updates prevent deadlocks; isolation of hot transactional inventory mutations from cold relational analytics.
- **Negative**: Requires document-model query planning; eventual consistency considerations across services; dual database technology stack (PostgreSQL + DynamoDB).
