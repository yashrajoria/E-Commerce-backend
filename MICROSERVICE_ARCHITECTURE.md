# ShopSwift — Microservice Architecture Documentation

Comprehensive architecture specification for the **ShopSwift** e-commerce backend platform.

---

## 1. Executive Overview

ShopSwift is an enterprise-grade e-commerce microservices platform built primarily with **Go 1.25** (multi-module workspace) and a **Python (FastAPI)** AI Agent service. The system is designed around an event-driven architecture using AWS services (S3, DynamoDB, SNS, SQS) fully emulated locally via **LocalStack**, alongside **PostgreSQL** for relational transactions and **Redis** for state caching, rate limiting, and distributed locking.

### Key Architectural Principles
- **API Gateway + BFF Pattern**: Gateway handles edge routing, rate limiting, correlation IDs, and JWT validation. The BFF (Backend-For-Frontend) handles domain aggregation and async checkout orchestration.
- **Polyglot Persistence**: 
  - **PostgreSQL**: Transactional entities (Users, Orders, Payments, Coupons, Notifications).
  - **DynamoDB**: High-throughput catalog, category adjacency graphs, and inventory stock.
  - **Redis**: Cart state, product catalog caching, gateway rate limits, and checkout locks.
  - **AWS S3**: Product media assets.
- **Asynchronous Event-Driven Processing**: Orders, payments, and notifications are processed asynchronously via SNS topics and SQS queues with built-in Dead Letter Queues (DLQs).
- **Strict Idempotency & Safety**: Multi-tier idempotency guarantees across API calls (Client SetNX lock), SQS message processing (`idempotency_key` columns), inventory updates (`ClientRequestToken`), and Stripe webhooks (`stripe_processed_events`).

---

## 2. System Architecture Diagram

```mermaid
flowchart TB
  subgraph Clients ["Clients & External Systems"]
    Web["Web Storefront / Mobile App"]
    StripeExt["Stripe API / Webhooks"]
  end

  subgraph EdgeLayer ["Edge Layer"]
    GW["API Gateway (:8080)<br/>Go / Gin + Redis"]
  end

  subgraph AggregationLayer ["Aggregation Layer"]
    BFF["BFF Service (:8088)<br/>Go / Gin"]
  end

  subgraph CoreServices ["Core Domain Microservices"]
    AUTH["Auth Service (:8081)"]
    USER["User Service (:8085)"]
    PROD["Product Service (:8082)"]
    CART["Cart Service (:8086)"]
    ORDER["Order Service (:8083)"]
    PAYMENT["Payment Service (:8087)"]
    INVENT["Inventory Service (:8084)"]
    PROMO["Promotion Service (:8090)"]
    SHIP["Shipping Service (:8091)"]
    NOTIF["Notification Service (:8092)"]
    AGENT["Agent Service (:8089)<br/>Python FastAPI"]
  end

  subgraph Storage ["Data Persistence Stores"]
    PG[("PostgreSQL (:5432)<br/>ecommerce DB")]
    DDB[("DynamoDB<br/>Products, Categories, Inventory")]
    REDIS[("Redis 7 (:6379)<br/>Cart, Locks, Cache, Limits")]
    S3[("AWS S3 / LocalStack<br/>shopswift bucket")]
  end

  subgraph Messaging ["Event Broker (SNS / SQS)"]
    SNS["SNS Topics<br/>order, payment, notification events"]
    SQS["SQS Queues & DLQs<br/>order-processing, payment-request, notification"]
  end

  %% HTTP Flows
  Web -->|HTTP / Cookies| GW
  GW -->|Proxy /bff| BFF
  GW -->|Proxy /auth| AUTH
  GW -->|Proxy /users| USER
  GW -->|Proxy /products| PROD
  GW -->|Proxy /cart| CART
  GW -->|Proxy /orders| ORDER
  GW -->|Proxy /payments| PAYMENT
  GW -->|Proxy /inventory| INVENT
  GW -->|Proxy /promotions| PROMO
  GW -->|Proxy /shipping| SHIP
  GW -->|Proxy /notifications| NOTIF
  GW -->|Proxy /agent| AGENT

  BFF -->|Checkout Lock SetNX| REDIS
  BFF -->|HTTP Aggregation| GW
  AGENT -->|Direct Call| BFF
  StripeExt -->|Webhook /stripe/webhook| GW

  %% Data Store Connections
  AUTH --> PG
  USER --> PG
  ORDER --> PG
  PAYMENT --> PG
  PROMO --> PG
  NOTIF --> PG

  PROD --> DDB
  PROD --> S3
  PROD --> REDIS
  INVENT --> DDB
  CART --> REDIS
  GW --> REDIS

  %% Async Event Connections
  CART -->|Publish order-events| SNS
  ORDER -->|Publish payment-request| SQS
  PAYMENT -->|Publish payment-events| SNS
  StripeExt <-->|API / Webhook| PAYMENT
  SNS --> SQS
  SQS -->|Consume| ORDER
  SQS -->|Consume| PAYMENT
  SQS -->|Consume| NOTIF
  ORDER -->|Reserve Stock| INVENT
```

---

## 3. Microservices Directory & Topology

| Service | Port | Technology | Primary Data Store | Responsibility Summary |
|:---|:---:|:---|:---|:---|
| **api-gateway** | `8080` | Go / Gin | Redis | Edge router, JWT validation, rate limiting, `X-Request-ID` correlation, client header sanitization. |
| **auth-service** | `8081` | Go / Gin | Postgres (`users`, `refresh_tokens`) | Identity authentication, token refresh, password hashing, admin bootstrapping. |
| **product-service** | `8082` | Go / Gin | DynamoDB + Redis + AWS S3 | Catalog items, categories, category-product adjacency graph, S3 image uploads, cache versioning. |
| **order-service** | `8083` | Go / Gin | Postgres (`orders`, `order_items`) + SQS | Order state machine, stock reservation integration, payment request dispatch. |
| **inventory-service** | `8084` | Go / Gin | DynamoDB (`Inventory`) | Real-time stock levels, idempotent reservations (`ClientRequestToken`), stock confirmations. |
| **user-service** | `8085` | Go / Gin | Postgres (`users`, `addresses`) | Customer profiles, address book management. |
| **cart-service** | `8086` | Go / Gin | Redis | Real-time user shopping cart storage, cart checkout event publishing. |
| **payment-service** | `8087` | Go / Gin | Postgres (`payments`, `stripe_processed_events`) | Stripe Checkout session creation, webhook processing, payment event publishing. |
| **bff-service** | `8088` | Go / Gin | Redis | Storefront aggregation (home, profile, checkout), checkout idempotency (`SetNX`), status polling. |
| **agent-service** | `8089` | Python / FastAPI | Stateless | AI-powered conversational backend assistant interacting with the BFF. |
| **promotion-service**| `8090` | Go / Gin | Postgres (`coupons`) | Coupon rules, atomic coupon usage limit checks and discounts. |
| **shipping-service** | `8091` | Go / Gin | In-Memory / Static JSON | Zone-based shipping rate calculations and delivery estimations. |
| **notification-service**| `8092` | Go / Gin | Postgres (`notification_logs`) + SQS | Async notification consumer (`notification-queue`), transactional emails. |

---

## 4. Data Architecture & Ownership Matrix

### 4.1 PostgreSQL (`ecommerce` database)
Single shared Postgres database instance with clear per-service table ownership boundary:

```
ecommerce/
 ├── users                   [Owned by auth-service (auth credentials) & user-service (profile)]
 ├── refresh_tokens          [Owned by auth-service]
 ├── addresses               [Owned by user-service]
 ├── orders                  [Owned by order-service]
 ├── order_items             [Owned by order-service]
 ├── payments                [Owned by payment-service]
 ├── stripe_processed_events [Owned by payment-service (webhook deduplication)]
 ├── coupons                 [Owned by promotion-service]
 └── notification_logs       [Owned by notification-service]
```

### 4.2 DynamoDB Tables

| Table Name | Environment Key | Partition Key (PK) | Sort Key (SK) / GSIs | Managing Service |
|:---|:---|:---|:---|:---|
| **Products** | `DDB_TABLE_PRODUCTS` | `id` (String) | GSIs: `sku-index` (`sku`), `featured-index` (`is_featured`, `created_at`) | `product-service` |
| **Categories** | `DDB_TABLE_CATEGORIES` | `id` (String) | GSI: `name-index` (`name`) | `product-service` |
| **ProductCategories** | `DDB_TABLE_PRODUCT_CATEGORIES` | `category_id` | `product_id` (GSI: `product-index` on `product_id`) | `product-service` |
| **Inventory** | `DDB_TABLE_INVENTORY` | `product_id` | — | `inventory-service` |

### 4.3 Redis Data Structure Usage

- **Cart (`cart-service`)**: Key `cart:{user_id}` storing JSON cart items.
- **Checkout Idempotency (`bff-service`)**: Key `checkout:lock:{idempotency_key}` using `SetNX` with TTL for atomic request locking and result caching.
- **Gateway Rate Limiting (`api-gateway`)**: Key `ratelimit:{ip/user_id}` managing window counters.
- **Product Caching (`product-service`)**: Product catalog queries cached with version-invalidation tags.

---

## 5. Event-Driven Messaging Architecture

### 5.1 SNS Topics & SQS Queue Map

```
[ Cart Service ] ---> ( SNS: order-events ) 
                             │
                             └───> [ SQS: order-processing-queue ] ---> [ Order Service ]
                                                                             │
                                                                             └───> [ SQS: payment-request-queue ] ---> [ Payment Service ]
                                                                                                                           │
[ Stripe Webhook ] ────────────────────────────────────────────────────────────────────────────────────────────────────────┘
                                                                                                                           │
[ Payment Service ] ---> ( SNS: payment-events ) 
                              │
                              ├───> [ SQS: payment-events-queue ] ---> [ Order Service (Confirm Order) ]
                              └───> [ SQS: notification-queue ]  ---> [ Notification Service (Send Email) ]
```

---

## 6. Critical Workflows & Sequence Diagrams

### 6.1 Asynchronous Checkout Flow

```mermaid
sequenceDiagram
  autonumber
  actor Client as Client / Storefront
  participant GW as API Gateway
  participant BFF as BFF Service
  participant Redis as Redis
  participant Cart as Cart Service
  participant SNS as SNS (order-events)
  participant SQS_Order as SQS (order-processing-queue)
  participant Order as Order Service
  participant Invent as Inventory Service
  participant SQS_Pay as SQS (payment-request-queue)
  participant Payment as Payment Service
  participant Stripe as Stripe API

  Client->>GW: POST /bff/checkout (Header: Idempotency-Key)
  GW->>BFF: Forward Request + X-User-ID
  BFF->>Redis: SETNX checkout:lock:{key} "PENDING" TTL=60s
  alt Lock Acquired
    BFF->>Cart: POST /cart/checkout
    Cart->>SNS: Publish checkout.requested
    Cart-->>BFF: Return temporary order_id
    SNS->>SQS_Order: Route message
    SQS_Order->>Order: Consume checkout.requested
    Order->>Invent: Reserve Stock (ClientRequestToken)
    Order->>Order: Create Order (Status: PENDING)
    Order->>SQS_Pay: Enqueue payment request
    SQS_Pay->>Payment: Consume payment request
    Payment->>Stripe: Create Checkout Session
    Payment-->>Redis: Cache checkout_url for order_id
    BFF->>Payment: Poll GET /payments/order/{order_id}
    Payment-->>BFF: Return checkout_url
    BFF->>Redis: Update checkout:lock:{key} "SUCCESS"
    BFF-->>Client: 200 OK (checkout_url)
  else Duplicate Request
    BFF->>Redis: GET checkout:lock:{key}
    BFF-->>Client: Return cached status / result
  end
```

### 6.2 Payment Confirmation & Fulfillment Flow

```mermaid
sequenceDiagram
  autonumber
  actor Stripe as Stripe Webhook
  participant GW as API Gateway
  participant Payment as Payment Service
  participant PG as Postgres
  participant SNS as SNS (payment-events)
  participant SQS_Order as SQS (payment-events-queue)
  participant Order as Order Service
  participant SQS_Notif as SQS (notification-queue)
  participant Notif as Notification Service

  Stripe->>GW: POST /stripe/webhook
  GW->>Payment: Forward Webhook Payload
  Payment->>PG: Check stripe_processed_events (event.id)
  alt Event Already Processed
    Payment-->>Stripe: 200 OK (Ignored duplicate)
  else New Event
    Payment->>PG: Update Payment status = PAID
    Payment->>PG: Insert event.id into stripe_processed_events
    Payment->>SNS: Publish payment.succeeded
    Payment-->>Stripe: 200 OK ACK
    
    par Order Fulfillment
      SNS->>SQS_Order: Route payment.succeeded
      SQS_Order->>Order: Consume payment.succeeded
      Order->>PG: Update Order status = PAID / CONFIRMED
    and Customer Notification
      SNS->>SQS_Notif: Route notification-events
      SQS_Notif->>Notif: Consume notification event
      Notif->>PG: Log notification in notification_logs
      Notif->>Notif: Send Order Confirmation Email
    end
  end
```

---

## 7. Security, Authorization & Idempotency

### 7.1 Security & Auth Model
- **Authentication**: `auth-service` issues JWT access tokens and HTTP-only refresh tokens.
- **Gateway Injection**: `api-gateway` strips any client-provided identity headers (`X-User-ID`, `X-User-Role`), validates the JWT, and injects validated identity context headers into internal requests.
- **Role-Based Access Control (RBAC)**: Routes marked with admin privileges require `X-User-Role: admin`. Domain services re-verify roles for sensitive operations (e.g., product creation, admin user creation).

### 7.2 Idempotency Architecture
1. **API Level**: Client sends `Idempotency-Key` header to `/bff/checkout`. BFF uses Redis `SetNX` to lock concurrent requests.
2. **Order Creation**: Order Service verifies unique `idempotency_key` constraint on PostgreSQL `orders` table.
3. **Inventory Management**: Inventory Service utilizes DynamoDB conditional updates with `ClientRequestToken` for idempotent stock reservation and release.
4. **Stripe Webhooks**: Payment Service tracks processed Stripe event IDs in `stripe_processed_events` table before executing downstream event dispatch.

---

## 8. Local Development & Operational Tools

### 8.1 Prerequisites
- Docker & Docker Compose
- Go 1.25+
- LocalStack (emulating AWS S3, DynamoDB, SNS, SQS)

### 8.2 Environment Startup
```bash
cd backend
cp .env.example .env
./scripts/dev-up.sh
```

### 8.3 Database Migrations & Data Seeding
```bash
# Run SQL Migrations
cd backend
./scripts/migrate.sh up

# Seed Demo Data (Products, Categories, Inventory)
./scripts/seed_demo_data.sh
```

---

## 9. Author & Maintainer

- **Yash Rajoria** — [GitHub](https://github.com/yashrajoria) · [LinkedIn](https://www.linkedin.com/in/yashrajoria)
