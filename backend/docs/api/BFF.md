# Admin API (`/bff/admin/*`)

> The `bff-service` process is deleted. The `/bff/admin/*` path prefix is
> retained as the admin/analytics contract (admin UI + agent tools); the
> gateway forwards each path straight to the owning service. All routes
> require JWT + `admin` role. Storefront `/bff/*` paths no longer exist —
> use the domain routes (`/products`, `/cart`, `/orders`, …) directly.

Base URL: http://localhost:8080 (via API Gateway)

## Aggregation

- GET /bff/admin/dashboard — KPI/top-products/activity summary. Computed in
  `order-service` (`admin` package): order stats/counts/recents in-process,
  user and product totals over the mesh.

## Direct proxies (no aggregation)

- GET /bff/admin/reports/sales → `order-service` `GET /orders/admin/stats`
- GET /bff/admin/reports/users → `identity-service` `GET /users`
- GET /bff/admin/reports/inventory → `catalog-service` `GET /inventory`
- GET|POST /bff/admin/products, PUT|POST|DELETE /bff/admin/products/* → `catalog-service` `/products/*`
- GET /bff/admin/products/presign, POST /bff/admin/products/:id/images/presign → `catalog-service` presign
- GET|POST /bff/admin/categories, PUT|DELETE /bff/admin/categories/* → `catalog-service` `/categories/*`
- GET /bff/admin/users, PUT|DELETE /bff/admin/users/* → `identity-service` `/users/*`
- POST /bff/admin/users → `identity-service` `POST /auth/admin/users`
- GET /bff/admin/orders → `order-service` `GET /orders/admin/`
- GET|PUT /bff/admin/orders/* → `order-service` `/orders/*`
- GET /bff/admin/inventory, PUT /bff/admin/inventory/* → `catalog-service` `/inventory/*`
- GET|POST|PUT|DELETE /bff/admin/coupons* → `order-service` `/coupons*`
- GET /bff/admin/notifications, GET /bff/admin/notifications/log → `notification-service`

## Removed storefront paths

`GET /bff/home`, `GET /bff/profile`, `POST /bff/checkout`, `GET|POST|DELETE /bff/cart*`,
`GET /bff/products*`, `GET /bff/categories*`, `GET|POST /bff/payment*`,
`POST /bff/promotions/validate`, `POST|GET /bff/auth/*`, `PUT /bff/users/profile`,
`POST /bff/users/change-password` — deleted with bff-service.

Migration: `POST /cart/checkout` (with `Idempotency-Key`) returns
`{order_id, PENDING}`; poll `GET /payment/status/by-order/:order_id` for the
Stripe `checkout_url`.
