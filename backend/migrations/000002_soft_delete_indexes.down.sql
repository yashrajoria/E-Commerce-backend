-- Idempotent down.
DROP INDEX IF EXISTS idx_notification_logs_user_id;
DROP INDEX IF EXISTS idx_coupons_active_expires;
DROP INDEX IF EXISTS idx_orders_status;
DROP INDEX IF EXISTS idx_payments_deleted_at;
DROP INDEX IF EXISTS idx_orders_deleted_at;
DROP INDEX IF EXISTS idx_addresses_deleted_at;
