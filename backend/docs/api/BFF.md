# BFF Service API

Base URL: http://localhost:8088

Security: uses API Gateway auth (cookie-based). Supported header: `Idempotency-Key` (optional) for POST idempotency.

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

- **Idempotency**: The `/bff/checkout` endpoint uses a distributed Redis lock to prevent duplicate orders. If a second request is received while the first is still processing, it will return a `409 Conflict`.
- **Aggregation**: This service calls Product, Cart, Order, Promotion, Shipping, and Payment services to provide unified responses to the storefront.
- Responses reference shared schemas in `docs/openapi.yaml`.

- For examples, open `docs/openapi.yaml` and inspect request/response schemas under `components.schemas`.
