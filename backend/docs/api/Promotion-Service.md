# Promotion Service API

Base URL: `http://localhost:8090` (Service Internal)
Gateway Prefix: `/promotion` (Admin Only)

## Endpoints

### Storefront (Internal Only)
- **POST /coupons/validate**: Validates a coupon code against a cart total.
    - **Request**: `{"code": "SUMMER10", "cart_total": 100.0}`
    - **Response**: `{"valid": true, "discount_amount": 10.0, "message": "Success"}`

### Admin (via API Gateway)
- **POST /coupons**: Create a new coupon.
- **GET /coupons**: List all coupons (paginated).
- **GET /coupons/{code}**: Get specific coupon details.
- **DELETE /coupons/{code}**: Deactivate a coupon.

## Business Rules
- **Usage Limits**: Enforced atomically at the database level.
- **Expiry**: Coupons automatically invalidate after their `expires_at` date.
- **Minimum Order**: Coupons can have a `min_order_value` required for activation.
