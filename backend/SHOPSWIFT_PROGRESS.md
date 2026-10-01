# ShopSwift Execution Plan — Progress (100% Complete)

Source plan: audit-corrected roadmap. All phases verified and tested as of 2026-09-23.

## Context

Original 20-phase roadmap was audit-corrected. Outbox infrastructure, payment state machine, Stripe webhook idempotency, order-side payment consumer idempotency, checkout consumer idempotency, DLQ/retry, refresh-token reuse detection, rate limiting, and the AI agent-service have all been audited, fixed, verified, and backed by automated tests.

---

## Phase Status Summary

| Phase | Description | Status | Verification Summary |
|---|---|---|---|
| **Phase A** | Close outbox dual-write gap | ✅ **DONE** | Transactional outbox pattern implemented in `order-service` and `payment-service`. Outbox events verified `published` in DB. |
| **Phase B** | Verify security checklist | ✅ **DONE** | Verified: Downstream direct bypass rejected with 401; admin routes reject non-admin with 403; strict rate limiting active (Redis); CORS allowlist active. |
| **Phase C** | Verify checkout flow end-to-end | ✅ **DONE** | Full E2E walk passed: Login → Add to Cart → Checkout (`8aa92970-b311-49f9-bbdd-fd12e4baa230`) → DynamoDB stock reservation → Stripe session → Stripe CLI webhook `checkout.session.completed` → Order transitioned to `paid` → Stock confirmed → `order_confirmed` notification delivered. |
| **Phase D** | Integration / failure tests | ✅ **DONE** | New Go tests added: `expired_token_test.go` (auth-service), `duplicate_checkout_test.go` (order-service). Webhook redelivery test passed. 100% of Go unit/integration test suites pass across all services. |
| **Phase E** | Harden / verify the AI agent | ✅ **DONE** | Tool allowlist (`TOOL_REGISTRY`), Pydantic validation, role-based execution gate, mutation human-in-the-loop confirmation (`POST /agent/mutations/{id}/confirm`). Added `tests/test_security.py`. All 32 pytest tests pass in container. |
| **Phase F** | Database & performance audit | ✅ **DONE** | `EXPLAIN ANALYZE` verified: hot queries on `orders`, `payments`, `outbox_events`, `payment_outbox_events` all execute via index scans in < 0.1 ms using partial btree indexes. |
| **Phase G** | Docs, ADRs, cleanup, demo | ✅ **DONE** | 5 ADRs created (`001-bff.md` through `005-ai-agent-architecture.md`). `openapi.yaml` updated with agent mutation endpoints. 5–10 min demo script written (`docs/demo-script.md`). |

---

## Phase Details & Verification Evidence

### Phase A — Close the outbox dual-write gap — DONE
1. `order-service`: Checkout consumer enqueues `payment_request` and `order_created` events into `outbox_events` inside the same GORM transaction as the order creation (`CreateWithOutbox`).
2. `payment-service`: Enqueues `payment_succeeded` / `payment_failed` events into `payment_outbox_events` within the atomic status transition transaction (`UpdateIfStatusNotInWithOutbox`).
3. Outbox publishers poll and claim events using lease locks, publish to SQS/SNS, and mark them `published`.
4. DB Verification:
   ```sql
   SELECT id, event_type, status, destination_type FROM outbox_events;
   -- c1e6a741... | order_created   | published | sns
   -- a88c527f... | payment_request | published | sqs
   SELECT id, event_type, status, destination_type FROM payment_outbox_events;
   -- 426adc30... | payment_succeeded | published | sns
   ```

### Phase B — Verify Phase-1 security checklist — DONE
- **Service-to-service auth**: `internalauth.Require()` gates internal endpoints with `INTERNAL_SERVICE_TOKEN`. Direct bypass without token yields `401 Unauthorized`.
- **Admin role enforcement**: Verified `/bff/admin/products` returns `403 Forbidden` (`{"error":"Admin role required"}`) when accessed with standard user JWT.
- **Unauthenticated requests**: Verified returns `401 Unauthorized` (`{"error":"Missing authentication token"}`).
- **CORS configuration**: Gateway enforces `defaultAllowedOrigins` allowlist. Unlisted origins (e.g. `https://evil.com`) receive no `Access-Control-Allow-Origin` header; valid origins (`http://localhost:3000`) receive matching header.
- **Rate limiting**: `StrictRateLimiter` applies a 5 req/min Redis fixed-window counter on `/auth/login` and `/auth/register`.

### Phase C — Checkout flow end-to-end — DONE
- Executed live against Docker stack using `alice.johnson@shopswift-demo.test`:
  1. `POST /auth/login` → 200 OK, session cookie issued.
  2. `POST /cart/add` → 200 OK, items added in Redis.
  3. `POST /cart/checkout` with `Idempotency-Key` → 200 OK, order `8aa92970-b311-49f9-bbdd-fd12e4baa230` created at `pending_payment`.
  4. DynamoDB transactional reservation succeeded for 5 items.
  5. `GET /payment/status/by-order/:order_id` → returned Stripe checkout URL and session `cs_test_...`.
  6. Triggered Stripe webhook via CLI:
     ```bash
     docker exec backend-stripe-cli-1 stripe trigger checkout.session.completed \
       --add checkout_session:metadata.order_id=8aa92970-b311-49f9-bbdd-fd12e4baa230 \
       --add checkout_session:metadata.user_id=11111111-1111-4111-8111-111111111101
     ```
  7. Payment transitioned to `succeeded`.
  8. Order transitioned to `paid` with `completed_at` timestamp.
  9. Stock confirmed in DynamoDB for 5 items.
  10. `notification-service` received and processed `order_confirmed` event.

### Phase D — Integration / failure tests — DONE
- **Auth token expiry / tampering**: Added `services/auth-service/services/expired_token_test.go`:
  - `TestExpiredAccessToken_IsRejected` — PASS
  - `TestExpiredRefreshToken_IsRejected` — PASS
  - `TestWrongTokenType_IsRejected` — PASS
  - `TestTamperedSignature_IsRejected` — PASS
- **Duplicate checkout idempotency**: Added `services/order-service/services/duplicate_checkout_test.go`:
  - `TestDuplicateCheckoutRequest_SecondCallIsIdempotent` — PASS (simulates duplicate SQS delivery; first creates order, second hits duplicate key and recovers via `FindByIdempotencyKey` without side effects).
- **Stripe webhook redelivery**: `TestStripeWebhook_ConcurrentRedelivery` — PASS.
- **Full Go Test Suite**: 100% pass across all 12 services and common packages (`api-gateway`, `auth-service`, `order-service`, `payment-service`, `cart-service`, `bff-service`, `promotion-service`, `notification-service`, `shipping-service`, `inventory-service`, `product-service`, `common/internalauth`).

### Phase E — Harden/verify the AI agent — DONE
- **Safety boundaries enforced**:
  - LLM restricted to `TOOL_REGISTRY` allowlist; unknown tools dropped before execution.
  - Pydantic argument validation (`schemas.py`, `validator.py`); out-of-range, negative quantities, or invalid types rejected.
  - Max 5 tools per turn enforced (`MAX_TOOLS`).
  - Role-based tool access (`can_execute_tool`).
  - Mutating tools require two-phase commit: proposals are stored as `pending_confirmation` in `agent_audit_log` and only executed after admin approval via `POST /agent/mutations/{request_id}/confirm`.
- **Test coverage**:
  - Added `services/agent-service/tests/test_security.py` (allowlist enforcement, role gate, max tools truncation, injection rejection, no raw DB/shell tools).
  - All **32 pytest tests pass** in `backend-agent-service-1` container.

### Phase F — Database & performance audit — DONE
- Analyzed hot query execution plans via `EXPLAIN ANALYZE`:
  - `orders WHERE user_id = ? ORDER BY created_at DESC`: Index scan via `idx_orders_user_id` (0.096 ms).
  - `payments WHERE order_id = ?`: Index scan via `idx_payments_order_id` (0.039 ms).
  - `outbox_events WHERE status = 'pending' AND available_at <= now()`: Index scan via `idx_outbox_events_claimable` (0.017 ms).
  - `payment_outbox_events WHERE status = 'pending' AND available_at <= now()`: Index scan via `idx_payment_outbox_events_claimable` (0.010 ms).
- GORM relationships use eager preloading (`Preload("OrderItems")`), eliminating N+1 queries.
- High-frequency cart operations isolated in Redis; inventory transactional operations isolated in DynamoDB.

### Phase G — Docs, ADRs, cleanup, demo — DONE
- Created 5 Architecture Decision Records in `docs/adr/`:
  - `001-bff.md` — Backend-For-Frontend (BFF) Pattern.
  - `002-outbox-pattern.md` — Transactional Outbox Pattern for Asynchronous Events.
  - `003-stripe-checkout.md` — Stripe Hosted Checkout & Webhook Idempotency.
  - `004-dynamodb-inventory.md` — DynamoDB for Atomic Inventory Reservation.
  - `005-ai-agent-architecture.md` — AI Agent Architecture, Safety Boundaries & Human-in-the-Loop Mutations.
- Created `docs/demo-script.md`: Comprehensive 5–10 minute CLI walkthrough covering auth, catalog, cart, checkout saga, Stripe trigger, outbox verification, and AI assistant interaction.
- Updated `docs/openapi.yaml`: Added `/agent/mutations/{request_id}`, `/agent/mutations/{request_id}/confirm`, and `/agent/tools`.

---

## Test Execution Commands

```bash
# 1. Run all Go tests across all services (all pass)
for dir in api-gateway pkg/aws services/auth-service services/order-service \
           services/payment-service services/cart-service services/bff-service \
           services/promotion-service services/notification-service \
           services/shipping-service services/inventory-service \
           services/product-service services/common/internalauth; do
  (cd backend/$dir && go test ./... -v)
done

# 2. Run Python agent tests inside container (32 passed)
docker exec backend-agent-service-1 python -m pytest tests/ -v
```
