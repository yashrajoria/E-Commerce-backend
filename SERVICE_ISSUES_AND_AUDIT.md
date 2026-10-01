# ShopSwift Microservices — Service-by-Service Engineering Audit & Issues Report

> **Current topology (2026-10-01, `develop@a986387` + fixes):** 5 domain services + gateway + agent.
> - `api-gateway` (:8080) — edge router, JWT, rate limiting, header sanitization
> - `identity-service` (:8081) — auth, users, addresses (merged ex-`auth-service` + `user-service`)
> - `catalog-service` (:8082) — products, categories, inventory, cart (merged ex-`product` + `inventory` + `cart`)
> - `order-service` (:8083) — orders, coupons, shipping, payments (merged ex-`promotion` + `shipping` + `payment`)
> - `notification-service` (:8092) — async email/SMS consumer
> - `agent-service` (:8089) — Python FastAPI AI assistant, own `agent_audit_log` pool only
> - `bff-service` — **deleted** (§8 below is archive only).
>
> Old §2/§9 → `services/identity-service/{auth,users}/`; §3/§6/§7 → `services/catalog-service/{,inventory/,cart/}`;
> §4/§5/§10/§11 → `services/order-service/{,payment/,promotion/,shipping/}`.
> `POST /auth/internal/revoke-tokens` HTTP hop is gone (in-process). `order → promotion` HTTP hop is gone
> (in-process). `payment-request-queue` retained as durable buffer for in-process payment consumer.
> `order_reservations` init race, coupon atomic increment, cart N+1 batch, internal-auth fail-closed,
> prompt length cap, correlation forwarding — all shipped before this pass.

---

## Executive Summary & Severity Matrix

| Severity | Definition | Count Identified (original) | Open after 2026-10-01 pass |
|:---:|:---|:---:|:---:|
| **HIGH** | Security, data corruption, stock races, missing rollbacks. | 15 | 0 (1 by-design degraded start, now fail-fast in prod) |
| **MEDIUM** | N+1, scans, validation, retries. | 14 | 0 fixed in code; 3 accepted as future (shipping DB rates, SES, NTP) |
| **LOW** | Edge cases, correlation, pool tuning. | 13 | 0 |

---

## 1. API Gateway (`backend/api-gateway`)

- 1.1 CORS wildcard fallback — ✅ RESOLVED (explicit allowlist, `main.go:53-54`).
- 1.2 Plain HTTP internal auth URL — ✅ BY DESIGN (in-cluster DNS, `AUTH_SERVICE_URL` overridable, `middlewares/jwt.go:38-41`).
- 1.3 Refresh transport — ✅ RESOLVED (`MaxIdleConns:100/PerHost:10`, 5s timeout, singleflight, `middlewares/jwt.go:21-28`).
- Rate limiting — ✅ Redis fixed-window, fails open (`middlewares/rate_limiter.go:34-44`); defense-in-depth identity login/register limit.
- Header sanitization — ✅ strips `X-User-*` + cookies, injects from JWT (`utils/forwarder.go:120-165`, test `forwarder_identity_test.go`).
- Admin gate — ✅ `AdminRoleMiddleware` + `newAdminGroup` (`middlewares/jwt.go:223-235`).

## 2. Identity Service (`backend/services/identity-service`) — ex-§2 Auth + ex-§9 User

- 2.1 Verification brute-force — ✅ RESOLVED (SHA256 at rest, 5 attempts + 15m lock, `auth/services/verification.go:29-32`).
- 2.2 Password validation — ✅ RESOLVED (72-byte bcrypt guard `auth/controllers/auth_controller.go:127-129`, complexity `services/common/password/validator.go`, enforced Register/AdminCreate/Change/Bootstrap).
- 2.3 Cookies — ✅ RESOLVED (all HttpOnly `auth_controller.go:86-90`; clear-cookie HttpOnly mismatch fixed 2026-10-01).
- Lockout — ✅ 10 attempts + 15m, dummy-hash anti-enumeration (`auth_service.go:74-98`).
- Soft-delete — ✅ GORM `DeletedAt` auto-filter, no `Raw()` (`repository/repository.go:87`).
- Phone 409 — ✅ duplicate → 409 (`users/controllers/user_controller.go:83-85`).
- Address ownership (§9.1) — N/A (no address CRUD routes; `users/routes/routes.go:11-23` only profile/password/admin list; model exists, endpoints absent).
- Tests — ✅ FIXED 2026-10-01: added `auth/services/register_login_test.go` (Register success/duplicate, Login success/wrong/unverified/unknown). Prior 4 files were edge-only.

## 3. Catalog Service (`backend/services/catalog-service`) — ex-product (§3) + inventory (§6) + cart (§7)

- 3.1 Product filter scan — ✅ RESOLVED (GSI/adjacency routing, `repository/dynamo_adapter.go`).
- 3.2 Magic-byte image check — ✅ FIXED 2026-10-01: `controllers/validator.go` sniffs 512B via `http.DetectContentType` + allow-list (`IsValidImageType`, `ValidateImageContentType`); `services/product_services_ddb.go:uploadImageFromURL` rejects non-image + stores detected type; `presigned_url_handler.go` validates filename ext + content-type (direct-to-S3 can't sniff bytes — ext+type gate + S3 ContentType).
- 3.3 Cache invalidation — ✅ hybrid (`controllers/cache_manager.go:117-152`: global `products:version` bump + per-product `DEL product:detail:{id}`; Create→global, Update/Delete→per-product, bulk→global).
- 6.1 Order-reservations race — ✅ RESOLVED (`if_not_exists` + same-txn write).
- 6.2 Admin audit — ✅ FIXED 2026-10-01: `SetStock` + `UpdateStock` both zap `[AUDIT]` with `X-User-ID` (`inventory/controllers/inventory_controller.go:56-61,87-92`); admin-only via `inventory/middleware/rbac.go`.
- 6.3 Inventory pagination — ✅ FIXED 2026-10-01: `inventory/services/inventory_service.go:ListAllStock` walks Scan cursors to skip `(page-1)*pageSize` (was returning page 1 for every page); repo `ListAll` already supports `Limit/ExclusiveStartKey`.
- 7.1 Cart N+1 checkout — ✅ RESOLVED (batch validate); inventory `CheckStock` N+1 — ✅ FIXED (prior pass: `BatchGet` 100-key chunks + `UnprocessedKeys` retry, `inventory/repository/inventory_repository.go:103`, service single `BatchGet`).
- 7.2 Cart quantity — ✅ FIXED 2026-10-01: per-item `max=999` kept + cumulative `>999` → 400 + distinct `>100` → 400 (`cart/controllers/cart_controller.go:93-140`).
- 7.3 Idempotency key format — ✅ RESOLVED (SHA256 hex, `userID:hex`, `cart_controller.go:295-308`).

## 4. Order Service (`backend/services/order-service`) — ex-order (§4) + payment (§5) + promotion (§10) + shipping (§11)

- 4.1 Compensating saga — ✅ RESOLVED (release on Create failure, `services/sqs_checkout_consumer.go:274-283`).
- 4.2 Order status race — ✅ RESOLVED (conditional `UPDATE WHERE status`, side-effects gated, `sqs_payment_consumer.go`, test `sqs_payment_consumer_redelivery_test.go`).
- 4.3 Queue fallbacks — ✅ FIXED 2026-10-01: degraded start kept for dev/LocalStack (Warn), **fail-fast in production** (`ENV=production` Fatal on missing checkout/payment-request/ORDER_SNS, `main.go`). `SQSOutboxPublisher.Publish` resolves queue names via `GetQueueUrl` (`pkg/aws/sqs.go:35-43`), so `Destination:"payment-request-queue"` is valid.
- DB pools — ✅ FIXED (prior pass + this pass): `database/db.go` + `payment/database/db.go` (legacy, unused — `main.go` uses single `database.DB`) now `20/10/2h` matching identity. Note: `payment/database` is dead code, consider deletion.
- 5.1 Webhook window — ✅ RESOLVED (fulfill-then-record so Stripe retries on failure, `payment/controllers/payment_webhook.go:35-63`).
- 5.2 Idempotency validation — ✅ FIXED 2026-10-01: async regex `^[a-zA-Z0-9_:\-]{1,128}$` (error string now includes `:`, `payment/services/payment_request_consumer_sqs.go:89`); sync `POST /orders` validates 400 (`controllers/order_controllers.go:48-56`, shared `services/idempotency.go`); `CreateCheckoutSession` sync remains record-first (Stripe session mint is inherently non-idempotent — async claim path is the guard).
- 5.3 Stripe retries — ✅ FIXED 2026-10-01: explicit `SetMaxNetworkRetries(2)` (`payment/services/stripe_client.go:NewStripeService`; SDK default is already 2 — now pinned + documented); Stripe `IdempotencyKey` forwarded on async path (`CreateCheckoutSessionWithIdempotency`, consumer passes `req.IdempotencyKey`).
- 5.4 Duplicate publish — ✅ RESOLVED (atomic `UpdateIfStatusNotIn[WithOutbox]`, `payment/repository/payment_repository.go:96-127`).
- Timestamps — ✅ FIXED 2026-10-01: webhook `now`, `updatePaymentStatus`, `InitiatePayment`, `CheckoutEvent.Timestamp`, `CancelOrder`, `sqs_checkout_consumer` Created/Updated, coupon event timestamp all `time.Now().UTC()`.
- 10.1 Coupon counter — ✅ RESOLVED (atomic `UPDATE ... used_count<usage_limit` + `RowsAffected`, `promotion/repository/coupon_repository.go:53-68`).
- 10.2 Coupon clock — ✅ UTC everywhere (`coupon_service.go:65,107`); NTP/leeway + DB-clock compare accepted as future (host-clock reliance documented).
- 10.3 Coupon pagination — ✅ RESOLVED (`page/limit`, max 100, meta, `promotion/controllers/coupon_controller.go:91-141`).
- 11.1 Shipping fallback — ✅ RESOLVED (malformed country → 422 `ErrUnserviceableDestination`, `shipping/providers/internal_provider.go:29-31`, `shipping/services/shipping_service.go:44-47`).
- 11.2 Rate matrix — ⚠️ ACCEPTED FUTURE: still static in-code (`internal_provider.go:38-57`); 2026-10-01 fix wires `STORE_CURRENCY` through (`NewShippingService(..., currency)`, uppercase, `main.go:158`) + weight `<=0` → 400 (`ErrInvalidWeight`). DB-backed dynamic rates require migration — not done.
- 11.3 Delivery strings — ⚠️ ACCEPTED (static `5/2/1d` by design).

## 5. Notification Service (`backend/services/notification-service`) — §12

- 12.1 DLQ — ✅ RESOLVED (delete-only-on-success, `consumer/sqs_consumer.go:111-122`; DLQ `maxReceiveCount:3` in `localstack/init/ready.d/10-bootstrap-resources.sh:228-254`).
- 12.2 Template escaping — ✅ RESOLVED (`html/template`, `services/notification_service.go:8,89-144`; zero `ReplaceAll`).
- 12.3 SMTP pool — ✅ FIXED 2026-10-01: pooled authenticated connection (`sender/smtp_sender.go`: `Mail/Rcpt/Data` + `Reset` reuse, `Noop` health, `STARTTLS`, mutex-serialized, one-shot `SendMail` fallback; `SentAt` UTC). SES migration accepted as future.

## 6. Agent Service (`backend/services/agent-service`) — §13

- 13.1 Prompt sanitization — ✅ RESOLVED (`max_length=2000` + whitespace collapse, `app/agent/schemas.py:17-23`).
- 13.2 Internal secret — ✅ RESOLVED (fail-closed 503, `services/common/internalauth/token.go:32-53`).
- 13.3 Correlation — ✅ RESOLVED (`X-Correlation-ID` forwarded, `app/tools/executor.py:33-76`, `main.py:28-35`).
- Audit trail — ✅ IMPLEMENTED (contradicts old handoff "planned"): `migrations/000009_agent_audit_log.{up,down}.sql`, `app/audit/repository.py` (pending→confirmed→executed), `app/agent/executor.py:70-107`, `app/api/routes.py:99-156`, tests `tests/test_mutations.py`. Agent holds asyncpg pool (`app/core/db.py:10-14`) scoped to `agent_audit_log` only.
- Env ints — ✅ RESOLVED (`_safe_float/_safe_int`, `app/core/config.py:3-30`).

## 7. BFF (`bff-service`) — §8 ARCHIVE (deleted `6bd1717`)

- 8.1-8.3 apply to deleted code only. Storefront uses domain routes; `/bff/admin/*` survive on gateway as forwards; dashboard aggregation → `order-service/admin/`.

## 8. Infra (§15)

- 15.1 Redis URL — ✅ FIXED 2026-10-01: all entrypoints tolerate `redis://` URI + bare `host:port` (gateway `api-gateway/main.go:210-220`, catalog `main.go:68-73`, cart `cart/database/redis.go:11-18` — was `Fatal` on bare, now fallback). `.env.example` aligned to `redis://redis:6379`. Lib split noted: gateway/common `go-redis/v8`, catalog `v9` (`catalog-service/README.md` "unified on v9" is aspirational).
- 15.2 Stripe CLI path — ✅ no mismatch (`docker-compose.yml:339` → `api-gateway:8080/stripe/webhook` → `orderBase+/stripe/webhook`, `api-gateway/routes/routes.go:117`, `order-service/payment/routes/routes.go:23`; zero `/payments` hits).
- 15.3 Agent env ints — ✅ RESOLVED (see §6).

---

## Action Plan (updated 2026-10-01 — all code items done)

- [x] Order DB pools (both `database/db.go`)
- [x] Inventory `BatchGet` N+1
- [x] Identity register/login happy-path tests + `ProvisionUser` nil-DB test hook
- [x] Image magic-byte + presigned ext gate + bulk-URL allow-list
- [x] Cart cumulative/distinct caps
- [x] Inventory pagination + `UpdateStock` audit
- [x] Redis URL tolerance (3 entrypoints) + `.env.example`
- [x] Order idempotency sync validation + error-string colon + shared validator
- [x] Stripe retries pinned + idempotency-key forwarding
- [x] `time.Now().UTC()` sweep (webhook, helpers, orders, coupons)
- [x] Shipping weight 400 + `STORE_CURRENCY` wiring + prod queue fail-fast
- [x] SMTP pooled connection + clear-cookie HttpOnly
- [ ] Future (accepted): shipping rates DB, SES, NTP/DB-clock coupon compare
