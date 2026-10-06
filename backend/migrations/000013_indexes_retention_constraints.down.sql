ALTER TABLE notification_events DROP CONSTRAINT IF EXISTS chk_notification_events_status;
ALTER TABLE notification_logs DROP CONSTRAINT IF EXISTS chk_notification_logs_status;
ALTER TABLE notification_logs DROP CONSTRAINT IF EXISTS chk_notification_logs_channel;
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_email_lower;
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_role;

DROP INDEX IF EXISTS idx_payment_outbox_events_published;
DROP INDEX IF EXISTS idx_outbox_events_published;
DROP INDEX IF EXISTS idx_stripe_processed_events_processed_at;
DROP INDEX IF EXISTS idx_notification_events_created_at;
-- notification_events.created_at stays: the Go model writes it.

CREATE INDEX IF NOT EXISTS idx_orders_status ON orders (status);
DROP INDEX IF EXISTS idx_orders_status_created;
DROP INDEX IF EXISTS idx_orders_user_created;
DROP INDEX IF EXISTS idx_order_items_product_id;
