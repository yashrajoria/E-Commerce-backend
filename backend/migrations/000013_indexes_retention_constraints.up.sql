-- Query indexes, purge support and remaining constraint gaps.
-- Guarded and idempotent like 000012 (works on baseline- and AutoMigrate-built DBs).

-- ---------------------------------------------------------------------------
-- 0. Drift repair (found by the schema drift tests: AutoMigrate on a database
--    built only by migrations changed these)
-- ---------------------------------------------------------------------------

-- Written by the Go model, never added by a migration.
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS revoked_at timestamptz;

-- Go int is 64-bit, so the models use bigint.
ALTER TABLE orders ALTER COLUMN amount TYPE bigint, ALTER COLUMN discount_amount TYPE bigint;
ALTER TABLE order_items ALTER COLUMN price TYPE bigint, ALTER COLUMN quantity TYPE bigint;
ALTER TABLE payments ALTER COLUMN amount TYPE bigint;

-- The notification models use text / bigint, and user_id is a plain string.
ALTER TABLE notification_logs
  ALTER COLUMN user_id TYPE text,
  ALTER COLUMN channel TYPE text,
  ALTER COLUMN status TYPE text,
  ALTER COLUMN retry_count TYPE bigint;
ALTER TABLE notification_events ALTER COLUMN status TYPE text;

-- ---------------------------------------------------------------------------
-- 1. Indexes
-- ---------------------------------------------------------------------------

-- Price-drop and per-product lookups filter order_items by product.
CREATE INDEX IF NOT EXISTS idx_order_items_product_id ON order_items (product_id);

-- Order history (per user, newest first) and admin dashboard (by status, by date).
-- The status index is covered by the (status, created_at) prefix.
CREATE INDEX IF NOT EXISTS idx_orders_user_created ON orders (user_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_orders_status_created ON orders (status, created_at) WHERE deleted_at IS NULL;
DROP INDEX IF EXISTS idx_orders_status;

-- ---------------------------------------------------------------------------
-- 2. Retention support (rows are deleted by each service's purge job)
-- ---------------------------------------------------------------------------

-- notification_events had no timestamp to purge by. AutoMigrate may have added
-- the column already (nullable), so normalise it either way.
ALTER TABLE notification_events ADD COLUMN IF NOT EXISTS created_at timestamptz;
UPDATE notification_events SET created_at = now() WHERE created_at IS NULL;
ALTER TABLE notification_events
  ALTER COLUMN created_at SET DEFAULT now(),
  ALTER COLUMN created_at SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_notification_events_created_at ON notification_events (created_at);
CREATE INDEX IF NOT EXISTS idx_stripe_processed_events_processed_at ON stripe_processed_events (processed_at);
CREATE INDEX IF NOT EXISTS idx_outbox_events_published ON outbox_events (published_at) WHERE status = 'published';
CREATE INDEX IF NOT EXISTS idx_payment_outbox_events_published ON payment_outbox_events (published_at) WHERE status = 'published';

-- ---------------------------------------------------------------------------
-- 3. CHECK constraints (NOT VALID, then VALIDATE; violations only NOTICE)
-- ---------------------------------------------------------------------------

DO $$
DECLARE r record;
BEGIN
  FOR r IN SELECT * FROM (VALUES
    ('users',               'chk_users_role',                $c$role IN ('user','admin')$c$),
    ('users',               'chk_users_email_lower',         'email = lower(email)'),
    ('notification_logs',   'chk_notification_logs_channel', $c$channel IN ('email','sms')$c$),
    ('notification_logs',   'chk_notification_logs_status',  $c$status IN ('sent','failed')$c$),
    ('notification_events', 'chk_notification_events_status',$c$status IN ('processing','delivered')$c$)
  ) AS t(tbl, name, expr)
  LOOP
    CONTINUE WHEN to_regclass(r.tbl) IS NULL;

    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = r.name AND conrelid = to_regclass(r.tbl)) THEN
      BEGIN
        EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I CHECK (%s) NOT VALID', r.tbl, r.name, r.expr);
      EXCEPTION WHEN undefined_column THEN
        RAISE NOTICE 'skipped %: column missing on %', r.name, r.tbl;
        CONTINUE;
      END;
    END IF;

    BEGIN
      EXECUTE format('ALTER TABLE %I VALIDATE CONSTRAINT %I', r.tbl, r.name);
    EXCEPTION WHEN check_violation THEN
      RAISE NOTICE '% has violating rows: enforced for new writes only. Fix data, then ALTER TABLE % VALIDATE CONSTRAINT %.', r.name, r.tbl, r.name;
    END;
  END LOOP;
END $$;
