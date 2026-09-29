# catalog-service (port `8082`)

Merged from `product-service` + `inventory-service` + `cart-service`. Owns
everything you can browse, stock, or put in a basket: catalog, categories,
S3 images, bulk import, stock levels and reservations, and the Redis cart.
Single binary on `:8082`, one shared Redis client, one DynamoDB client.

## Routes (unchanged from the pre-merge services)

- `GET /products`, `GET /products/:id`, admin `POST|PUT|DELETE /products/*`, presign + bulk endpoints
- `GET /products/internal/:id`, `POST /products/internal/batch-validate` (mesh, order/cart callers — order-service still calls these over HTTP)
- `GET|POST|PUT /categories/*`
- `GET /inventory`, `POST /inventory`, `PUT /inventory/:productId`, `GET /inventory/:productId` (public read), mesh `POST /inventory/check|reserve|release|confirm`
- `GET|POST|DELETE /cart/*`, `POST /cart/coupon`, `POST /cart/checkout`

## What the merge deleted

- `product → inventory` HTTP hop (`INVENTORY_SERVICE_URL`): quantity sync is an
  in-process call with identical fire-and-forget semantics.
- `cart → product` HTTP hop (`PRODUCT_SERVICE_URL` + mesh token): checkout
  validation calls `GetProductsInternal` in-process.
- Redis client split (`go-redis/v8` vs `v9`): unified on `v9`, one client.
- `cart/config` package: folded into root `Config` (`CartTTL`).

## What stayed on the wire

- `order-service → catalog` inventory reserve/release/confirm and product
  internal reads remain HTTP (separate binaries, mesh token required).
- `cart → SNS order-events` publish stays: the order-service boundary is real.

## Env

`PORT` (default `8082`), `JWT_SECRET` (required), `REDIS_URL`,
`DDB_TABLE_PRODUCTS|CATEGORIES|PRODUCT_CATEGORIES|INVENTORY`,
`AWS_S3_BUCKET`, `AWS_S3_PREFIX`, `ASSET_PUBLIC_BASE_URL`,
`ORDER_SNS_TOPIC_ARN`, `INTERNAL_SERVICE_TOKEN`, `BULK_STORAGE_DIR`.
