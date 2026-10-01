-- Reverse the Phase 1 reliability schema. IF EXISTS keeps rollback safe when
-- a partially applied or manually repaired database is encountered.
DROP TABLE IF EXISTS notification_events;
DO $$
BEGIN
	IF to_regclass('public.payments') IS NOT NULL THEN
		DROP INDEX IF EXISTS idx_payments_event_id;
		ALTER TABLE payments DROP COLUMN IF EXISTS event_id;
	END IF;
END
$$;
DROP TABLE IF EXISTS outbox_events;