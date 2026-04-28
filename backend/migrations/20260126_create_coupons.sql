-- Migration: create coupons table for promotion-service
-- Date: 2026-01-26

CREATE TABLE IF NOT EXISTS coupons (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code           VARCHAR(64) NOT NULL,
    type           VARCHAR(20) NOT NULL CHECK (type IN ('percentage', 'flat', 'freeshipping')),
    value          NUMERIC(10, 2) NOT NULL CHECK (value >= 0),
    min_order_value NUMERIC(10, 2) NOT NULL DEFAULT 0 CHECK (min_order_value >= 0),
    usage_limit    INTEGER NOT NULL DEFAULT 0,    -- 0 = unlimited
    used_count     INTEGER NOT NULL DEFAULT 0,
    expires_at     TIMESTAMPTZ NOT NULL,
    active         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at     TIMESTAMPTZ
);

-- Unique index on lowercased code for case-insensitive lookups
CREATE UNIQUE INDEX IF NOT EXISTS idx_coupons_code_lower
    ON coupons (LOWER(code))
    WHERE deleted_at IS NULL;

-- Index for looking up only active coupons quickly
CREATE INDEX IF NOT EXISTS idx_coupons_active ON coupons (active, expires_at);

-- Trigger to keep updated_at in sync
CREATE OR REPLACE FUNCTION trigger_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS set_coupons_updated_at ON coupons;
CREATE TRIGGER set_coupons_updated_at
  BEFORE UPDATE ON coupons
  FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
