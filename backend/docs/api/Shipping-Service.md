# Shipping Service API

Base URL: `http://localhost:8091` (service internal)  
Gateway prefix: `/shipping`

## Endpoints (runtime)

### Storefront / BFF

- **POST /shipping/rates** — Calculate shipping rates from weight and destination.
  - **Request**: `{"weight_kg": 2.5, "destination": {"country": "US", "postal_code": "94117"}}`
  - **Response**: list of `{provider, service_level, amount, ...}`

There are **no** shipment CRUD admin routes registered at runtime. A `shipments` SQL migration exists for future use but shipping-service does not connect to Postgres today.

## Calculation logic (internal provider)

Zone-based engine (`InternalDynamicProvider`):

- **Zone 1 (US)**: $5.00 base + $1.50/kg
- **Zone 2 (CA/MX)**: $12.00 base + $3.00/kg
- **Zone 3 (Other)**: $25.00 base + $7.50/kg
- Multipliers: **Standard** (1.0x), **Express** (1.8x), **Overnight** (3.2x)
