# Shipping Service

The shipping service calculates delivery rates from the request address, zone, cart contents, and configured provider data. It is a Go/Gin service on port `8091` with no runtime database.

## Responsibilities

- Authenticate rate requests.
- Resolve a destination to a shipping zone.
- Calculate available shipping methods, prices, and delivery estimates.
- Keep rate calculation deterministic and independent from order persistence.

## Architecture

```mermaid
flowchart LR
  Client[Checkout client] --> Gateway[API Gateway :8080]
  Gateway --> Shipping[Shipping Service :8091\nGo / Gin]
  Shipping --> Provider[In-process provider\nstatic/dynamic zone data]
  Shipping --> Response[Rates and estimates]
  Shipping -.-> Order[Order / BFF callers]
```

There is no Postgres write, shipment consumer, or shipping event publisher in the current runtime implementation. The `shipments` migration is reserved for future ownership and is not used by this service.

## Rate flow

```mermaid
sequenceDiagram
  actor Client
  participant Gateway
  participant Shipping
  participant Provider

  Client->>Gateway: POST /shipping/rates
  Gateway->>Shipping: Forward authenticated request
  Shipping->>Provider: Resolve zone and methods
  Provider-->>Shipping: Price and delivery estimate
  Shipping-->>Client: Rate options
```

## HTTP surface

| Route | Purpose | Access |
| --- | --- | --- |
| `POST /shipping/rates` | Calculate available shipping rates | Authenticated |

## Configuration and extension

Configure service port, provider data, and auth settings. Keep provider logic behind the provider interface so a live carrier API or persisted shipment model can be introduced without changing the HTTP contract.
