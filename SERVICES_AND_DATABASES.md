# ShopSwift — Microservices, Database & Infrastructure Specification

An exhaustive technical reference manual covering every microservice, database schema, caching key-space, event queue, API route, and infrastructure component in the **ShopSwift** backend repository.

---

## Table of Contents

1. [Architectural Topology & Matrix](#1-architectural-topology--matrix)
2. [Microservices Deep Dive (13 Services)](#2-microservices-deep-dive-13-services)
   - [2.1 API Gateway Service (`:8080`)](#21-api-gateway-service-8080)
   - [2.2 Auth Service (`:8081`)](#22-auth-service-8081)
   - [2.3 Product Service (`:8082`)](#23-product-service-8082)
   - [2.4 Order Service (`:8083`)](#24-order-service-8083)
   - [2.5 Inventory Service (`:8084`)](#25-inventory-service-8084)
   - [2.6 User Service (`:8085`)](#26-user-service-8085)
   - [2.7 Cart Service (`:8086`)](#27-cart-service-8086)
   - [2.8 Payment Service (`:8087`)](#28-payment-service-8087)
   - [2.9 BFF (Backend-For-Frontend) Service (`:8088`)](#29-bff-backend-for-frontend-service-8088)
   - [2.10 Agent Service (`:8089`)](#210-agent-service-8089)
   - [2.11 Promotion Service (`:8090`)](#211-promotion-service-8090)
   - [2.12 Shipping Service (`:8091`)](#212-shipping-service-8091)
   - [2.13 Notification Service (`:8092`)](#213-notification-service-8092)
3. [Database Architecture & Complete Schemas](#3-database-architecture--complete-schemas)
   - [3.1 PostgreSQL Database (`ecommerce`)](#31-postgresql-database-ecommerce)
   - [3.2 AWS DynamoDB Tables](#32-aws-dynamodb-tables)
   - [3.3 Redis Caching & State Key-Space](#33-redis-caching--state-key-space)
   - [3.4 AWS S3 Asset Storage](#34-aws-s3-asset-storage)
4. [Event-Driven Messaging (SNS & SQS)](#4-event-driven-messaging-sns--sqs)
5. [Infrastructure, Scripts & Environment Configurations](#5-infrastructure-scripts--environment-configurations)

---

## 1. Architectural Topology & Matrix

```
                                  ┌───────────────────┐
                                  │   Client / UI     │
                                  └─────────┬─────────┘
                                            │ HTTP / Cookies
                                  ┌─────────▼─────────┐
                                  │   API Gateway     │ (Port 8080, Gin, Redis Rate Limit)
                                  └─────────┬─────────┘
        ┌───────────────────────────────────┼───────────────────────────────────┐
        │ Proxy                             │ Proxy                             │ Proxy
┌───────▼───────┐                   ┌───────▼───────┐                   ┌───────▼───────┐
│  BFF Service  │ (:8088)           │  Auth Service │ (:8081)           │Product Service│ (:8082)
└───────┬───────┘                   └───────┬───────┘                   └───────┬───────┘
        │ SetNX Lock                        │ Postgres                          │ DynamoDB / S3 / Redis
┌───────▼───────┐                   ┌───────▼───────┐                   ┌───────▼───────┐
│ Redis 7 Cache │                   │ User Service  │ (:8085)           │Cart Service   │ (:8086)
└───────────────┘                   └───────┬───────┘                   └───────┬───────┘
                                            │ Postgres                          │ Redis / SNS
┌───────────────┐                   ┌───────▼───────┐                   ┌───────▼───────┐
│ Agent Service │ (:8089)           │ Order Service │ (:8083)           │Payment Service│ (:8087)
└───────┬───────┘                   └───────┬───────┘                   └───────┬───────┘
        │ Calls BFF                         │ Postgres / SQS                    │ Postgres / Stripe / SNS
        │                           ┌───────▼───────┐                   ┌───────▼───────┐
        │                           │Inventory Serv.│ (:8084)           │Promo Service  │ (:8090)
        │                           └───────┬───────┘                   └───────┬───────┘
        │                                   │ DynamoDB                          │ Postgres
        │                           ┌───────▼───────┐                   ┌───────▼───────┐
        │                           │Shipping Serv. │ (:8091)           │Notif Service  │ (:8092)
        └──────────────────────────>└───────────────┘                   └───────────────┘
                                      In-Memory                           Postgres / SQS
```

---

## 2. Microservices Deep Dive (13 Services)

---

### 2.1 API Gateway Service (`:8080`)

- **Port**: `8080`
- **Stack**: Go 1.25, Gin Framework, Redis Client
- **Primary Data Store**: Redis (`ratelimit:*`)
- **Main Responsibility**: Serves as the single edge entry point for all frontend traffic. Handles JWT authorization validation, HTTP cookie unwrapping, Redis-backed leaky/token-bucket rate limiting, correlation request ID injection (`X-Request-ID`), and client header sanitization.

#### Routing & Forwarding Table

| Endpoint Pattern | Upstream Destination | Auth Guard | Notes |
|:---|:---|:---|:---|
| `/health` | Self (`api-gateway`) | Public | Gateway liveness & readiness check |
| `/auth/*` | `http://auth-service:8081` | Mixed | Identity, login, refresh, admin user management |
| `/users/*` | `http://user-service:8085` | JWT Required | User profile and address book |
| `/products/*` | `http://product-service:8082` | Mixed | Catalog browsing (Public), CRUD (Admin) |
| `/categories/*` | `http://product-service:8082` | Mixed | Categories browsing (Public), CRUD (Admin) |
| `/cart/*` | `http://cart-service:8086` | JWT Required | Shopping cart management |
| `/orders/*` | `http://order-service:8083` | JWT Required | Customer order management |
| `/payments/*` | `http://payment-service:8087` | Mixed | Stripe checkout polling, Webhooks (Public) |
| `/inventory/*` | `http://inventory-service:8084` | Mixed | Stock checks (Public), Admin updates |
| `/promotions/*` | `http://promotion-service:8090` | Mixed | Coupon validations & Admin coupon rules |
| `/shipping/*` | `http://shipping-service:8091` | Public | Shipping rate queries |
| `/notifications/*` | `http://notification-service:8092` | JWT Required | Customer notification audit logs |
| `/bff/*` | `http://bff-service:8088` | Mixed | Aggregated storefront & async checkout |
| `/agent/*` | `http://agent-service:8089` | Public / JWT | AI conversational backend assistant |

#### Key Middleware
1. **Header Sanitizer**: Automatically deletes incoming `X-User-ID`, `X-User-Role`, `X-User-Email` from client HTTP requests to prevent header spoofing.
2. **Correlation ID**: Inspects incoming `X-Request-ID`; generates UUID if absent, and forwards it to upstream microservices.
3. **JWT Authenticator**: Extracts JWT from `Authorization: Bearer <token>` or `access_token` cookie; validates signature and injects `X-User-ID` and `X-User-Role` upstream.

---

### 2.2 Auth Service (`:8081`)

- **Port**: `8081`
- **Stack**: Go 1.25, Gin, GORM, Postgres
- **Primary Data Store**: PostgreSQL (`users`, `refresh_tokens` tables)
- **Main Responsibility**: Identity management, user registration, credential authentication (bcrypt), JWT access token generation, refresh token rotation/revocation, email verification code generation, and idempotent admin user initialization on startup.

#### Endpoints Specification

| Method | Path | Auth / Role | Request Body | Response / Purpose |
|:---|:---|:---|:---|:---|
| `POST` | `/auth/register` | Public | `{email, password, name, phone_number, store_name}` | `201 Created` — Registers new `role=user` |
| `POST` | `/auth/login` | Public | `{email, password}` | `200 OK` — Returns access token & sets HTTP refresh cookie |
| `POST` | `/auth/refresh` | Public / Refresh Cookie | `{refresh_token}` (or cookie) | `200 OK` — Issues new access token & rotates refresh token |
| `POST` | `/auth/logout` | JWT Required | `{refresh_token}` | `200 OK` — Revokes refresh token in database |
| `POST` | `/auth/verify-email` | JWT / Public | `{email, code}` | `200 OK` — Validates 6-digit verification code |
| `POST` | `/auth/admin/users` | Admin (`role=admin`) | `{email, password, name}` | `201 Created` — Invites/creates a new admin user |
| `GET` | `/auth/me` | JWT Required | — | `200 OK` — Returns authenticated identity info |
| `GET` | `/health` | Public | — | `200 OK` — Liveness status |

---

### 2.3 Product Service (`:8082`)

- **Port**: `8082`
- **Stack**: Go 1.25, Gin, AWS SDK v2 (DynamoDB & S3), Redis
- **Primary Data Store**: DynamoDB (`Products`, `Categories`, `ProductCategories`), AWS S3 (`shopswift` bucket), Redis
- **Main Responsibility**: Product catalog management, category hierarchies, category-product adjacency indexing, product image file upload to S3, and Redis versioned caching for high-speed read queries.

#### Endpoints Specification

| Method | Path | Auth / Role | Request Body / Query Params | Response / Purpose |
|:---|:---|:---|:---|:---|
| `GET` | `/products` | Public | `?category_id=&sku=&featured=&limit=&cursor=` | List products (Query GSI or Scan with filters) |
| `GET` | `/products/:id` | Public | — | Fetch single product by ID (Redis cached) |
| `POST` | `/products` | Admin (`role=admin`) | `{name, description, price, sku, brand, ...}` | Create product (Invalidates Redis catalog version) |
| `PUT` | `/products/:id` | Admin (`role=admin`) | `{name, description, price, ...}` | Update product details |
| `DELETE` | `/products/:id` | Admin (`role=admin`) | — | Soft-delete product |
| `POST` | `/products/:id/image` | Admin (`role=admin`) | Multipart Form File (`image`) | Upload image to S3 (`products/` prefix) and update URL |
| `GET` | `/categories` | Public | — | List all categories |
| `POST` | `/categories` | Admin (`role=admin`) | `{name, slug, description, parent_id}` | Create category |
| `GET` | `/categories/:id/products` | Public | `?limit=&cursor=` | Fetch products in category using `ProductCategories` GSI |

---

### 2.4 Order Service (`:8083`)

- **Port**: `8083`
- **Stack**: Go 1.25, Gin, GORM, Postgres, AWS SQS/SNS
- **Primary Data Store**: PostgreSQL (`orders`, `order_items` tables), SQS (`order-processing-queue`, `payment-events-queue`, `payment-request-queue`)
- **Main Responsibility**: Manages the complete lifecycle of customer orders. Consumes `checkout.requested` messages from SQS, calls Inventory Service to reserve stock, writes order and order items in Postgres, dispatches payment creation requests, and processes payment completion events.

#### Endpoints Specification

| Method | Path | Auth / Role | Request Body / Params | Purpose |
|:---|:---|:---|:---|:---|
| `POST` | `/orders` | JWT Required | `{items: [{product_id, quantity, price}], shipping_address_id, coupon_code}` | Direct synchronous order creation |
| `GET` | `/orders` | JWT Required | `?status=&page=&limit=` | Fetch user's orders |
| `GET` | `/orders/:id` | JWT Required | — | Fetch order details with order items |
| `PATCH` | `/orders/:id/cancel` | JWT Required | — | Cancel pending order & release stock |
| `GET` | `/orders/admin/all` | Admin | `?user_id=&status=` | Admin order search & reporting |

---

### 2.5 Inventory Service (`:8084`)

- **Port**: `8084`
- **Stack**: Go 1.25, Gin, AWS SDK v2 (DynamoDB)
- **Primary Data Store**: DynamoDB (`Inventory` table)
- **Main Responsibility**: Real-time stock level management, atomic stock reservation with idempotency token (`ClientRequestToken`), stock confirmation upon payment, and stock release upon cancellation.

#### Endpoints Specification

| Method | Path | Auth / Role | Request Body | Purpose |
|:---|:---|:---|:---|:---|
| `GET` | `/inventory/:product_id` | Public | — | Get stock level for a product |
| `POST` | `/inventory/reserve` | Internal / Order | `{order_id, items: [{product_id, quantity}], request_token}` | Reserve stock conditionally |
| `POST` | `/inventory/release` | Internal / Order | `{order_id, items: [{product_id, quantity}], request_token}` | Release reserved stock |
| `POST` | `/inventory/confirm` | Internal / Order | `{order_id, items: [{product_id, quantity}], request_token}` | Confirm reservation into sold stock |
| `POST` | `/inventory/set` | Admin | `{product_id, stock}` | Set absolute stock count |

---

### 2.6 User Service (`:8085`)

- **Port**: `8085`
- **Stack**: Go 1.25, Gin, GORM, Postgres
- **Primary Data Store**: PostgreSQL (`users` profile columns, `addresses` table)
- **Main Responsibility**: Management of user profiles (phone number, name, store name) and customer address books (shipping & billing addresses).

#### Endpoints Specification

| Method | Path | Auth / Role | Request Body | Purpose |
|:---|:---|:---|:---|:---|
| `GET` | `/users/profile` | JWT Required | — | Fetch authenticated user profile |
| `PUT` | `/users/profile` | JWT Required | `{name, phone_number, store_name}` | Update profile information |
| `GET` | `/users/addresses` | JWT Required | — | List all saved user addresses |
| `POST` | `/users/addresses` | JWT Required | `{type: "shipping"|"billing", street, city, state, postal_code, country}` | Add new address |
| `PUT` | `/users/addresses/:id` | JWT Required | `{street, city, state, ...}` | Update address |
| `DELETE` | `/users/addresses/:id` | JWT Required | — | Delete address |

---

### 2.7 Cart Service (`:8086`)

- **Port**: `8086`
- **Stack**: Go 1.25, Gin, Redis, AWS SNS
- **Primary Data Store**: Redis (`cart:{user_id}`)
- **Main Responsibility**: Manages fast, temporary customer shopping cart data stored strictly in Redis. Publishes `checkout.requested` messages to SNS topic `order-events` upon checkout initiation.

#### Endpoints Specification

| Method | Path | Auth / Role | Request Body | Purpose |
|:---|:---|:---|:---|:---|
| `GET` | `/cart` | JWT Required | — | Get user shopping cart contents |
| `POST` | `/cart/items` | JWT Required | `{product_id, quantity, price}` | Add item to cart |
| `PUT` | `/cart/items/:product_id` | JWT Required | `{quantity}` | Update item quantity |
| `DELETE` | `/cart/items/:product_id` | JWT Required | — | Remove item from cart |
| `DELETE` | `/cart` | JWT Required | — | Clear entire cart |
| `POST` | `/cart/checkout` | JWT Required | `{shipping_address_id, coupon_code}` | Triggers async checkout event to SNS |

---

### 2.8 Payment Service (`:8087`)

- **Port**: `8087`
- **Stack**: Go 1.25, Gin, GORM, Postgres, Stripe SDK, SQS/SNS
- **Primary Data Store**: PostgreSQL (`payments`, `stripe_processed_events` tables)
- **Main Responsibility**: Manages Stripe Checkout session creation, payment status tracking, Stripe webhook verification (`/stripe/webhook`), idempotency processing via event tracking, and SNS `payment-events` publishing.

#### Endpoints Specification

| Method | Path | Auth / Role | Request Body / Params | Purpose |
|:---|:---|:---|:---|:---|
| `POST` | `/payments/create-session` | Internal / Order | `{order_id, user_id, amount, currency, idempotency_key}` | Create Stripe Checkout Session |
| `GET` | `/payments/order/:order_id` | JWT / BFF | — | Query payment status and `checkout_url` |
| `POST` | `/payments/stripe/webhook` | Public (Stripe Signature) | Stripe Event Payload | Processes Stripe webhook & deduplicates via `event.id` |

---

### 2.9 BFF (Backend-For-Frontend) Service (`:8088`)

- **Port**: `8088`
- **Stack**: Go 1.25, Gin, Redis
- **Primary Data Store**: Redis (`checkout:lock:{idempotency_key}`)
- **Main Responsibility**: Aggregates responses from underlying microservices for single-page storefront loads (home catalog + cart + user profile). Orchestrates async checkout using Redis `SetNX` distributed locks to prevent double submission and polls Payment Service for the Stripe checkout URL.

#### Endpoints Specification

| Method | Path | Auth / Role | Request Body / Headers | Purpose |
|:---|:---|:---|:---|:---|
| `GET` | `/bff/home` | Public | — | Aggregated response: featured products, categories, cart summary |
| `POST` | `/bff/checkout` | JWT Required | Header: `Idempotency-Key`<br/>Body: `{shipping_address_id, coupon_code}` | Idempotent async checkout orchestrator |
| `GET` | `/bff/profile` | JWT Required | — | Aggregated user profile, addresses, and recent orders |

---

### 2.10 Agent Service (`:8089`)

- **Port**: `8089` (Internal container: `8000`)
- **Stack**: Python 3.11+, FastAPI, Uvicorn, Requests / HTTPX
- **Primary Data Store**: Stateless
- **Main Responsibility**: Provides a conversational AI assistant endpoint that interfaces directly with the BFF service to query catalog items, check cart status, and answer user e-commerce inquiries.

#### Endpoints Specification

| Method | Path | Auth / Role | Request Body | Purpose |
|:---|:---|:---|:---|:---|
| `POST` | `/agent/chat` | Public / JWT | `{prompt, context: {user_id}}` | Process natural language query and return agent response |
| `GET` | `/agent/health` | Public | — | Agent service health check |

---

### 2.11 Promotion Service (`:8090`)

- **Port**: `8090`
- **Stack**: Go 1.25, Gin, GORM, Postgres
- **Primary Data Store**: PostgreSQL (`coupons` table)
- **Main Responsibility**: Coupon code management, discount calculation (percentage or flat discount), minimum order value validation, and atomic usage count incrementing.

#### Endpoints Specification

| Method | Path | Auth / Role | Request Body | Purpose |
|:---|:---|:---|:---|:---|
| `POST` | `/promotions/validate` | JWT Required | `{code, order_amount}` | Validate coupon eligibility & calculate discount |
| `POST` | `/promotions/coupons` | Admin (`role=admin`) | `{code, discount_type, discount_value, usage_limit, expires_at}` | Create new coupon rule |
| `GET` | `/promotions/coupons` | Admin (`role=admin`) | — | List all coupons |
| `DELETE` | `/promotions/coupons/:id` | Admin (`role=admin`) | — | Deactivate coupon |

---

### 2.12 Shipping Service (`:8091`)

- **Port**: `8091`
- **Stack**: Go 1.25, Gin
- **Primary Data Store**: In-Memory / Static JSON Rate Rules
- **Main Responsibility**: Calculates estimated shipping rates and delivery dates based on shipping zone, destination country/postal code, and package weight/item count.

#### Endpoints Specification

| Method | Path | Auth / Role | Request Body | Purpose |
|:---|:---|:---|:---|:---|
| `POST` | `/shipping/rates` | Public | `{address_id, country, state, postal_code, items_count}` | Calculate available shipping options and rates |
| `GET` | `/health` | Public | — | Liveness check |

---

### 2.13 Notification Service (`:8092`)

- **Port**: `8092`
- **Stack**: Go 1.25, Gin, GORM, Postgres, AWS SQS
- **Primary Data Store**: PostgreSQL (`notification_logs` table), SQS (`notification-queue`)
- **Main Responsibility**: Consumes notification events from SQS queue, sends transactional emails (SMTP / LocalStack mock), and logs execution history in PostgreSQL.

#### Endpoints Specification

| Method | Path | Auth / Role | Request Body | Purpose |
|:---|:---|:---|:---|:---|
| `GET` | `/notifications` | JWT Required | `?page=&limit=` | Fetch notification history for user |
| `POST` | `/notifications/send-direct` | Admin / Internal | `{user_id, channel, template, payload}` | Trigger immediate notification |

---

## 3. Database Architecture & Complete Schemas

---

### 3.1 PostgreSQL Database (`ecommerce`)

PostgreSQL is managed using **`golang-migrate`** SQL scripts located in `backend/migrations/`.

#### 1. `users` Table
Stores user credentials, verification status, role, and profile parameters. Owned jointly by `auth-service` (auth credentials) and `user-service` (profile fields).

```sql
CREATE TABLE users (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email               text NOT NULL,
  password            text NOT NULL,
  name                text NOT NULL,
  email_verified      boolean NOT NULL DEFAULT false,
  verification_code   varchar(6),
  store_name          varchar(100),
  role                varchar(50) NOT NULL DEFAULT 'user',
  phone_number        text,
  billing_address_id  uuid,
  shipping_address_id uuid,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  deleted_at          timestamptz
);

CREATE UNIQUE INDEX idx_users_email ON users (email);
CREATE UNIQUE INDEX idx_users_phone_number ON users (phone_number);
CREATE INDEX idx_users_deleted_at ON users (deleted_at);
```

#### 2. `refresh_tokens` Table
Stores issued JWT refresh tokens. Owned by `auth-service`.

```sql
CREATE TABLE refresh_tokens (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  token_id    text NOT NULL,
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  revoked     boolean NOT NULL DEFAULT false,
  expires_at  timestamptz NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_refresh_tokens_token_id ON refresh_tokens (token_id);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens (user_id);
CREATE INDEX idx_refresh_tokens_expires_at ON refresh_tokens (expires_at);
```

#### 3. `addresses` Table
Stores customer shipping and billing addresses. Owned by `user-service`.

```sql
CREATE TABLE addresses (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  type        varchar(20) NOT NULL CHECK (type IN ('billing', 'shipping')),
  street      text NOT NULL,
  city        text NOT NULL,
  state       text NOT NULL,
  postal_code text NOT NULL,
  country     text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz
);

CREATE INDEX idx_addresses_user_id ON addresses (user_id);
CREATE INDEX idx_addresses_deleted_at ON addresses (deleted_at);
```

#### 4. `orders` Table
Stores customer order headers. Owned by `order-service`.

```sql
CREATE TABLE orders (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_number    text NOT NULL,
  idempotency_key varchar(128),
  user_id         uuid NOT NULL,
  amount          integer NOT NULL, -- Stored in smallest currency unit (e.g. cents)
  coupon_code     varchar(50),
  discount_amount integer NOT NULL DEFAULT 0,
  status          varchar(20) NOT NULL DEFAULT 'pending_payment',
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  deleted_at      timestamptz
);

CREATE UNIQUE INDEX idx_orders_order_number ON orders (order_number);
CREATE UNIQUE INDEX idx_orders_idempotency_key ON orders (idempotency_key);
CREATE INDEX idx_orders_user_id ON orders (user_id);
CREATE INDEX idx_orders_status ON orders (status);
CREATE INDEX idx_orders_deleted_at ON orders (deleted_at);
```

#### 5. `order_items` Table
Stores line items associated with orders. Owned by `order-service`.

```sql
CREATE TABLE order_items (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id   uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  product_id uuid NOT NULL,
  quantity   integer NOT NULL,
  price      integer NOT NULL
);

CREATE INDEX idx_order_items_order_id ON order_items (order_id);
```

#### 6. `payments` Table
Stores payment transactions and Stripe session links. Owned by `payment-service`.

```sql
CREATE TABLE payments (
  payment_id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  idempotency_key       varchar(128),
  order_id              uuid NOT NULL,
  user_id               uuid NOT NULL,
  amount                integer NOT NULL,
  currency              varchar(10) NOT NULL,
  status                varchar(20) NOT NULL, -- PENDING, PAID, FAILED, CANCELLED
  checkout_url          varchar(1024),
  stripe_payment_id     text,
  stripe_event_payload  jsonb,
  succeeded_at          timestamptz,
  failed_at             timestamptz,
  created_at            timestamptz NOT NULL DEFAULT now(),
  updated_at            timestamptz NOT NULL DEFAULT now(),
  deleted_at            timestamptz
);

CREATE UNIQUE INDEX idx_payments_idempotency_key ON payments (idempotency_key);
CREATE UNIQUE INDEX idx_payments_stripe_payment_id ON payments (stripe_payment_id);
CREATE INDEX idx_payments_order_id ON payments (order_id);
CREATE INDEX idx_payments_user_id ON payments (user_id);
CREATE INDEX idx_payments_deleted_at ON payments (deleted_at);
```

#### 7. `stripe_processed_events` Table
Tracks processed Stripe webhook event IDs to guarantee idempotent webhook processing. Owned by `payment-service`.

```sql
CREATE TABLE stripe_processed_events (
  event_id     text PRIMARY KEY,
  event_type   text NOT NULL,
  processed_at timestamptz NOT NULL DEFAULT now()
);
```

#### 8. `coupons` Table
Stores promotional code rules and usage counts. Owned by `promotion-service`.

```sql
CREATE TABLE coupons (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code             varchar(64) NOT NULL,
  discount_type    varchar(32) NOT NULL, -- percentage, fixed_amount
  discount_value   numeric NOT NULL,
  min_order_value  numeric,
  usage_limit      integer,
  used_count       integer NOT NULL DEFAULT 0,
  expires_at       timestamptz,
  active           boolean NOT NULL DEFAULT true,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_coupons_code ON coupons (code);
CREATE UNIQUE INDEX idx_coupons_code_lower ON coupons (LOWER(code));
CREATE INDEX idx_coupons_active_expires ON coupons (active, expires_at);
```

#### 9. `notification_logs` Table
Audit logs for system emails and notifications. Owned by `notification-service`.

```sql
CREATE TABLE notification_logs (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid,
  channel     varchar(32), -- email, sms, push
  template    varchar(128),
  status      varchar(32), -- SENT, FAILED
  payload     jsonb,
  error       text,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_notification_logs_user_id ON notification_logs (user_id);
```

---

### 3.2 AWS DynamoDB Tables

Managed by AWS SDK v2 in `product-service` and `inventory-service`.

#### 1. `Products` Table (`DDB_TABLE_PRODUCTS`)
- **Partition Key (PK)**: `id` (String / UUID)
- **Secondary Indexes (GSIs)**:
  - `sku-index`: HASH `sku` (String) — Used for fast barcode/SKU lookup via `Query`.
  - `featured-index`: HASH `is_featured` ("true"/"false"), RANGE `created_at` (String) — Used for homepage catalog queries via `Query`.
- **Attributes**: `id`, `name`, `description`, `price`, `sku`, `brand`, `is_featured`, `image_url`, `stock`, `created_at`, `updated_at`, `deleted_at`.

#### 2. `Categories` Table (`DDB_TABLE_CATEGORIES`)
- **Partition Key (PK)**: `id` (String / UUID)
- **Secondary Index (GSI)**:
  - `name-index`: HASH `name` (String)
- **Attributes**: `id`, `name`, `slug`, `description`, `parent_id`, `created_at`, `updated_at`.

#### 3. `ProductCategories` Table (`DDB_TABLE_PRODUCT_CATEGORIES`)
Category-to-product adjacency table to support many-to-many relationships cleanly without large DynamoDB scans.
- **Partition Key (PK)**: `category_id` (String)
- **Sort Key (SK)**: `product_id` (String)
- **Secondary Index (GSI)**:
  - `product-index`: HASH `product_id`, RANGE `category_id`

#### 4. `Inventory` Table (`DDB_TABLE_INVENTORY`)
- **Partition Key (PK)**: `product_id` (String)
- **Attributes**:
  - `product_id` (String)
  - `stock` (Number) — Total unreserved stock count available
  - `reserved` (Number) — Stock currently held in active checkout flows
  - `order_reservations` (Map) — Map of `order_id` to reservation status and quantity. Updated via atomic conditional DynamoDB expressions (`attribute_not_exists` / `ClientRequestToken`).

---

### 3.3 Redis Caching & State Key-Space

| Key Format | Type | Managing Service | TTL | Description |
|:---|:---:|:---|:---:|:---|
| `cart:{user_id}` | Hash / JSON | `cart-service` | 7 Days | User active shopping cart items |
| `checkout:lock:{idempotency_key}` | String | `bff-service` | 60 Seconds | `SetNX` lock to prevent duplicate checkout requests |
| `ratelimit:{ip_or_user_id}` | String | `api-gateway` | 1 Minute | Counter for API Gateway rate limiting |
| `product:cache:{product_id}` | JSON | `product-service` | 1 Hour | Cached product catalog item |
| `catalog:version` | Integer | `product-service` | Persistent | Incremented on catalog mutations to invalidate cache |

---

### 3.4 AWS S3 Asset Storage

- **Bucket Name**: `shopswift` (`AWS_S3_BUCKET`)
- **Object Prefix**: `products/` (`AWS_S3_PREFIX`)
- **File Upload Handler**: `product-service` handles multipart image uploads, assigns a unique UUID filename, uploads to S3, and returns the public CDN / LocalStack object URL: `http://localhost:4566/shopswift/products/{uuid}.png`.

---

## 4. Event-Driven Messaging (SNS & SQS)

```
                       ┌──────────────────────┐
                       │   Cart / BFF Service │
                       └──────────┬───────────┘
                                  │ Publish: checkout.requested
                       ┌──────────▼───────────┐
                       │  SNS: order-events   │
                       └──────────┬───────────┘
                                  │ SQS Subscription
                       ┌──────────▼───────────┐
                       │ SQS: order-processing│ ───> [ Order Service ]
                       └──────────────────────┘           │
                                                          │ Enqueue
                                               ┌──────────▼───────────┐
                                               │ SQS: payment-request │ ───> [ Payment Service ]
                                               └──────────────────────┘           │
                                                                                  │ Publish: payment.succeeded
                                               ┌──────────────────────┐           │
                                               │ SNS: payment-events  │ <─────────┘
                                               └──────────┬───────────┘
                                                          │
                        ┌─────────────────────────────────┴─────────────────────────────────┐
                        │ SQS Subscription                                                  │ SQS Subscription
             ┌──────────▼───────────┐                                            ┌──────────▼───────────┐
             │ SQS: payment-events  │ ───> [ Order Service ]                     │ SQS: notification    │ ───> [ Notification Service ]
             └──────────────────────┘      (Mark Order Paid)                     └──────────────────────┘      (Send Email & Audit Log)
```

### Event Payload Examples

#### 1. `checkout.requested` Event
```json
{
  "event_id": "evt_109283019283091",
  "event_type": "checkout.requested",
  "user_id": "8f3b2a11-5d9c-4b6e-8a12-9c3f4e5d6a7b",
  "idempotency_key": "idemp_checkout_991823",
  "shipping_address_id": "3a1b2c3d-4e5f-6a7b-8c9d-0e1f2a3b4c5d",
  "coupon_code": "SUMMER20",
  "timestamp": "2026-07-26T00:39:00Z"
}
```

#### 2. `payment.succeeded` Event
```json
{
  "event_id": "evt_payment_88231029",
  "event_type": "payment.succeeded",
  "order_id": "c1f2e3d4-5a6b-7c8d-9e0f-1a2b3c4d5e6f",
  "user_id": "8f3b2a11-5d9c-4b6e-8a12-9c3f4e5d6a7b",
  "stripe_payment_id": "pi_3MtwBwLkdIwHu7ix28a301",
  "amount": 4999,
  "currency": "usd",
  "timestamp": "2026-07-26T00:39:15Z"
}
```

---

## 5. Infrastructure, Scripts & Environment Configurations

### 5.1 LocalStack Initialization Scripts (`backend/localstack/`)
When LocalStack boots up via Docker Compose, it automatically runs initializers to create:
- S3 Bucket: `shopswift`
- DynamoDB Tables: `Products`, `Categories`, `ProductCategories`, `Inventory` (with GSIs `sku-index`, `featured-index`, `name-index`, `product-index`)
- SNS Topics: `order-events`, `payment-events`, `auth-events`, `promotion-events`, `notification-events`
- SQS Queues & DLQs: `order-processing-queue`, `payment-request-queue`, `payment-events-queue`, `notification-queue`, `promotion-order-queue`

### 5.2 Key Shell Scripts (`backend/scripts/`)
- `dev-up.sh`: Boots the complete Docker Compose + LocalStack container cluster.
- `migrate.sh`: Executes golang-migrate SQL scripts against PostgreSQL. Usage: `./scripts/migrate.sh up` or `./scripts/migrate.sh down 1`.
- `seed_demo_data.sh`: Populates DynamoDB tables with sample product catalog items, categories, adjacency links, and inventory stock.

---

## 6. Summary

This document represents the definitive technical specification for the **ShopSwift** e-commerce backend microservices system. It covers every component, database column, API endpoint, and async message queue across the entire codebase.
