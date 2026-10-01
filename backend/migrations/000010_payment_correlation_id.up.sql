ALTER TABLE payments ADD COLUMN IF NOT EXISTS correlation_id VARCHAR(128);
CREATE INDEX IF NOT EXISTS idx_payments_correlation_id ON payments (correlation_id);