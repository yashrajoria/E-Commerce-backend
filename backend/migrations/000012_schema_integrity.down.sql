-- Reverts constraints, indexes and coupon_usages. Does NOT undo the drift
-- repairs (coupons column rename, notification_logs bigint id, shipments.order_id
-- uuid, added columns): the Go models depend on the repaired shape.
-- Restoring the full unique indexes fails if a soft-deleted user/coupon now
-- shares an email/phone/code with a live one; remove those rows first.

DROP TABLE IF EXISTS coupon_usages;

DROP INDEX IF EXISTS idx_payments_one_succeeded_per_order;

ALTER TABLE coupons DROP CONSTRAINT IF EXISTS chk_coupons_usage;
ALTER TABLE coupons DROP CONSTRAINT IF EXISTS chk_coupons_percentage;
ALTER TABLE coupons DROP CONSTRAINT IF EXISTS chk_coupons_values;
ALTER TABLE coupons DROP CONSTRAINT IF EXISTS chk_coupons_type;
ALTER TABLE payments DROP CONSTRAINT IF EXISTS chk_payments_amount;
ALTER TABLE payments DROP CONSTRAINT IF EXISTS chk_payments_status;
ALTER TABLE order_items DROP CONSTRAINT IF EXISTS chk_order_items_values;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS chk_orders_amounts;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS chk_orders_status;

ALTER TABLE agent_audit_log DROP CONSTRAINT IF EXISTS agent_audit_log_confirmed_by_fkey;
ALTER TABLE agent_audit_log DROP CONSTRAINT IF EXISTS agent_audit_log_user_id_fkey;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_shipping_address_id_fkey;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_billing_address_id_fkey;
ALTER TABLE shipments DROP CONSTRAINT IF EXISTS shipments_order_id_fkey;
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_user_id_fkey;
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_order_id_fkey;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_coupon_id_fkey;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_user_id_fkey;

DROP INDEX IF EXISTS idx_orders_coupon_id;
ALTER TABLE orders DROP COLUMN IF EXISTS coupon_id;

-- Back to the baseline index shapes.
DROP INDEX IF EXISTS idx_coupons_code;
CREATE UNIQUE INDEX IF NOT EXISTS idx_coupons_code ON coupons (code);
CREATE UNIQUE INDEX IF NOT EXISTS idx_coupons_code_lower ON coupons (LOWER(code));
CREATE INDEX IF NOT EXISTS idx_coupons_active_expires ON coupons (active, expires_at);

DROP INDEX IF EXISTS idx_users_phone_number;
DROP INDEX IF EXISTS idx_users_email;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users (email);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_phone_number ON users (phone_number);
