# BFF Service API

Base URL: http://localhost:8088

Security: uses API Gateway auth (cookie-based).  
**Checkout requires** header `Idempotency-Key` (Redis SetNX). Checkout is **async**: cart publishes to SNS → order/payment SQS consumers → BFF polls payment for Stripe `checkout_url`.

Endpoints (concise):

- GET /bff/home — Home page data (products + categories)
- GET /bff/profile — Profile + orders aggregation
- POST /bff/auth/register — Register a user
- POST /bff/auth/login — Login (sets auth cookies)
- POST /bff/auth/logout — Logout (clear cookies)
- POST /bff/auth/refresh — Refresh access token (uses refresh cookie)
- GET /bff/auth/status — Get auth status
- POST /bff/auth/verify-email — Verify email

- GET /bff/products — Product list (query: page, perPage, filters: categoryId, price range, sort)
- GET /bff/products/{id} — Product detail
- GET /bff/categories — Category tree

- GET /bff/cart — Current cart
- POST /bff/cart/add — Add items to cart (supports `Idempotency-Key`)
- DELETE /bff/cart/remove/{product_id} — Remove item from cart
- DELETE /bff/cart/clear — Clear cart
- POST /bff/cart/checkout — Checkout cart (uses **Redis SetNX** for idempotency)
- POST /bff/checkout — Primary checkout endpoint (uses **Redis SetNX** for idempotency)

Notes:

- **Idempotency**: `/bff/checkout` uses Redis SetNX. Concurrent duplicate keys return `409 Conflict`. Downstream order/payment also store `idempotency_key`.
- **Aggregation**: Calls gateway/services for Product, Cart, Order, Promotion, Shipping, and Payment. Does not create orders synchronously via a single POST `/orders` in the happy path.
- **Health**: `GET /health`, `GET /health/live`, `GET /health/ready`.
- Responses reference shared schemas in `docs/openapi.yaml`.

- For examples, open `docs/openapi.yaml` and inspect request/response schemas under `components.schemas`.
