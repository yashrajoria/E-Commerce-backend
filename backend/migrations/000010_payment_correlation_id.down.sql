DROP INDEX IF EXISTS idx_payments_correlation_id;
ALTER TABLE payments DROP COLUMN IF EXISTS correlation_id;