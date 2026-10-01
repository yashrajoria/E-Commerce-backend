# ShopSwift Roadmap

New features and enhancements to build next, in priority order. Not a
wishlist — each item ships with a test proving the failure mode it claims
to fix. Don't claim an item done on CV/PR until its test exists and passes.

Related: [SERVICE_ISSUES_AND_AUDIT.md](SERVICE_ISSUES_AND_AUDIT.md) (known
bugs/gaps to fix) · [backend/docs/best-practices-and-gaps.md](backend/docs/best-practices-and-gaps.md)
(what's already shipped).

## Phase 1 — Reliability (complete)

- [x] Payment webhook redelivery idempotency (`payment_webhook_redelivery_test.go`)
- [x] Order SQS consumer redelivery idempotency (`sqs_payment_consumer_redelivery_test.go`)
- [x] Outbox pattern on order-service: `orders` write + `outbox_events` write in one
      transaction, lease-safe publisher polls/publishes to SQS/SNS, stable event IDs,
      retries, and consumer deduplication. Failure-mode tests cover publisher crash
      after publish, retries, lease expiry, concurrent claims, rollback, and duplicate
      idempotency keys.
- [x] DLQ + redrive verification for the LocalStack/Terraform queue topology. The
      repeatable `backend/scripts/verify_sqs_dlq.sh --exercise` check verifies policy,
      poison-message routing, and redrive back to source for all five source queues.

Phase 1 verification completed with focused race-enabled Go tests, Terraform
`fmt`/`validate`, and live LocalStack DLQ/redrive checks. Focused coverage reports
include the new reliability paths; unrelated legacy packages are not part of the
Phase 1 coverage target.

## Phase 2 — AI agent hardening

- [x] Typed tool schema for agent-service (no raw DB/HTTP access from LLM) —
      `services/agent-service/app/tools/schemas.py` + `ToolSpec.params_model`
      validated in `app/tools/validator.py` before any handler runs.
- [x] Split read tools (`get_sales`, `get_inventory`, `get_orders`) from mutating tools
      (`create_restock_request`, `cancel_order`) — `READ_TOOLS`/`MUTATING_TOOLS` in
      `app/tools/registry.py`; `cancel_order` backed by a new admin
      `PUT /orders/:id/cancel` in order-service.
- [x] Mutation confirmation step: agent proposes, admin approves, then executes —
      `POST /agent/mutations/{request_id}/confirm` in `app/api/routes.py`.
- [x] Audit trail: user_id, request_id, prompt, tool, arguments, confirmation,
      result, timestamp, status — `agent_audit_log` Postgres table
      (migration `000009`) via `app/audit/repository.py`.

## Phase 3 — Polish

- [x] Correlation ID propagation check: gateway → BFF → service → SQS → consumer
      (confirm it's end-to-end, not just gateway-in)
- [ ] Fix items in SERVICE_ISSUES_AND_AUDIT.md Phase 1 (security/integrity) before
      starting anything above marked optional

## Explicitly not doing

Kubernetes, Kafka, more microservices, RAG, storefront chatbot, DB-per-service
split, removing DynamoDB. No payoff for this project's scale — see
best-practices-and-gaps.md "Intentional deviations".
