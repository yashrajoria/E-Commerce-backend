-- Historical reference script (not run by golang-migrate).
-- Idempotent: safe to re-apply manually against an existing DB.
-- Date: 2026-01-26
--
-- NOTE: Column names here (`type`/`value`) predate the baseline schema
-- (`discount_type`/`discount_value`). Prefer migrations/000001_baseline.up.sql
-- for new environments.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS coupons (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code            VARCHAR(64) NOT NULL,
    type            VARCHAR(20) NOT NULL,
    value           NUMERIC(10, 2) NOT NULL,
    min_order_value NUMERIC(10, 2) NOT NULL DEFAULT 0,
    usage_limit     INTEGER NOT NULL DEFAULT 0,
    used_count      INTEGER NOT NULL DEFAULT 0,
    expires_at      TIMESTAMPTZ NOT NULL,
    active          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_coupons_code_lower
    ON coupons (LOWER(code))
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_coupons_active ON coupons (active, expires_at);

DO $$
BEGIN
  ALTER TABLE coupons
    ADD CONSTRAINT coupons_type_check
    CHECK (type IN ('percentage', 'flat', 'freeshipping'));
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
  ALTER TABLE coupons
    ADD CONSTRAINT coupons_value_check
    CHECK (value >= 0);
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
  ALTER TABLE coupons
    ADD CONSTRAINT coupons_min_order_value_check
    CHECK (min_order_value >= 0);
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

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
