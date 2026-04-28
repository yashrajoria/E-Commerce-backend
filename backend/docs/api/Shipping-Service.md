# Shipping Service API

Base URL: `http://localhost:8091` (Service Internal)
Gateway Prefix: `/shipping`

## Endpoints

### Storefront / BFF
- **POST /shipping/rates**: Calculates shipping rates based on weight and destination.
    - **Request**: `{"weight_kg": 2.5, "destination": {"country": "US", "postal_code": "94117", ...}}`
    - **Response**: List of `{"provider": "Internal", "service_level": "Standard", "amount": 8.75, ...}`

### Admin (via API Gateway)
- **GET /shipments/{id}**: Get shipment status.
- **GET /shipments/order/{order_id}**: Get shipment by order ID.
- **PATCH /shipments/{id}**: Update shipment status (tracking code, etc.).

## Calculation Logic (Free Internal Provider)
The service uses a local zone-based calculation engine:
- **Zone 1 (US)**: $5.00 base + $1.50/kg
- **Zone 2 (CA/MX)**: $12.00 base + $3.00/kg
- **Zone 3 (Other)**: $25.00 base + $7.50/kg
- Multipliers: **Standard** (1.0x), **Express** (1.8x), **Overnight** (3.2x)
