-- Soft-delete + filter indexes.
-- Idempotent: every statement uses IF NOT EXISTS.

CREATE INDEX IF NOT EXISTS idx_addresses_deleted_at ON addresses (deleted_at);
CREATE INDEX IF NOT EXISTS idx_orders_deleted_at ON orders (deleted_at);
CREATE INDEX IF NOT EXISTS idx_payments_deleted_at ON payments (deleted_at);

CREATE INDEX IF NOT EXISTS idx_orders_status ON orders (status);
CREATE INDEX IF NOT EXISTS idx_coupons_active_expires ON coupons (active, expires_at);
CREATE INDEX IF NOT EXISTS idx_notification_logs_user_id ON notification_logs (user_id);
