-- Migration 000022_subscription_plans.up.sql
-- Plans, entitlements, subscriptions, and usage records

-- 1. plans
CREATE TABLE IF NOT EXISTS plans (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            varchar(50) NOT NULL,
    slug            varchar(50) NOT NULL UNIQUE,
    price_monthly   integer NOT NULL DEFAULT 0,
    price_yearly    integer NOT NULL DEFAULT 0,
    currency        varchar(10) NOT NULL DEFAULT 'GBP',
    is_active       boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- 2. plan_entitlements
CREATE TABLE IF NOT EXISTS plan_entitlements (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id      uuid NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
    feature_key  varchar(100) NOT NULL,
    limit_value  integer,
    enabled      boolean NOT NULL DEFAULT true
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_plan_entitlements_plan_feature
    ON plan_entitlements (plan_id, feature_key);

-- 3. subscriptions
CREATE TABLE IF NOT EXISTS subscriptions (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id               uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    plan_id                uuid NOT NULL REFERENCES plans(id),
    status                 varchar(30) NOT NULL DEFAULT 'active',
    stripe_subscription_id text,
    stripe_customer_id     text,
    current_period_start   timestamptz,
    current_period_end     timestamptz,
    cancel_at_period_end   boolean NOT NULL DEFAULT false,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_subscriptions_store_id ON subscriptions (store_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_stripe_sub ON subscriptions (stripe_subscription_id);

-- 4. usage_records
CREATE TABLE IF NOT EXISTS usage_records (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id     uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    feature_key  varchar(100) NOT NULL,
    usage_count  integer NOT NULL DEFAULT 0,
    period_start timestamptz NOT NULL,
    period_end   timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_usage_records_store_period
    ON usage_records (store_id, feature_key, period_start);

-- 5. Seed default plans
INSERT INTO plans (name, slug, price_monthly, price_yearly) VALUES
    ('Free', 'free', 0, 0),
    ('Pro', 'pro', 2900, 29000),
    ('Business', 'business', 7900, 79000)
ON CONFLICT (slug) DO NOTHING;

-- Seed entitlements for Free plan
INSERT INTO plan_entitlements (plan_id, feature_key, limit_value, enabled)
SELECT p.id, e.feature_key, e.limit_value, e.enabled
FROM plans p
CROSS JOIN (VALUES
    ('max_stores', 1, true),
    ('max_products', 50, true),
    ('max_staff', 1, true),
    ('basic_analytics', NULL, true),
    ('advanced_analytics', NULL, false),
    ('ai_assistant', NULL, false),
    ('custom_domain', NULL, false),
    ('data_export_basic', NULL, true),
    ('data_export_advanced', NULL, false),
    ('api_access', NULL, false)
) AS e(feature_key, limit_value, enabled)
WHERE p.slug = 'free'
ON CONFLICT (plan_id, feature_key) DO NOTHING;

-- Seed entitlements for Pro plan
INSERT INTO plan_entitlements (plan_id, feature_key, limit_value, enabled)
SELECT p.id, e.feature_key, e.limit_value, e.enabled
FROM plans p
CROSS JOIN (VALUES
    ('max_stores', 3, true),
    ('max_products', 1000, true),
    ('max_staff', 5, true),
    ('basic_analytics', NULL, true),
    ('advanced_analytics', NULL, true),
    ('ai_assistant', 100, true),
    ('custom_domain', NULL, true),
    ('data_export_basic', NULL, true),
    ('data_export_advanced', NULL, true),
    ('api_access', NULL, false)
) AS e(feature_key, limit_value, enabled)
WHERE p.slug = 'pro'
ON CONFLICT (plan_id, feature_key) DO NOTHING;

-- Seed entitlements for Business plan
INSERT INTO plan_entitlements (plan_id, feature_key, limit_value, enabled)
SELECT p.id, e.feature_key, e.limit_value, e.enabled
FROM plans p
CROSS JOIN (VALUES
    ('max_stores', 10, true),
    ('max_products', NULL, true),
    ('max_staff', 25, true),
    ('basic_analytics', NULL, true),
    ('advanced_analytics', NULL, true),
    ('ai_assistant', 500, true),
    ('custom_domain', NULL, true),
    ('data_export_basic', NULL, true),
    ('data_export_advanced', NULL, true),
    ('api_access', NULL, true)
) AS e(feature_key, limit_value, enabled)
WHERE p.slug = 'business'
ON CONFLICT (plan_id, feature_key) DO NOTHING;

-- Backfill default store with active Free plan subscription
DO $$
DECLARE
    def_store_id uuid;
    free_plan_id uuid;
BEGIN
    SELECT id INTO def_store_id FROM stores WHERE slug = 'default-store' LIMIT 1;
    SELECT id INTO free_plan_id FROM plans WHERE slug = 'free' LIMIT 1;

    IF def_store_id IS NOT NULL AND free_plan_id IS NOT NULL THEN
        IF NOT EXISTS (SELECT 1 FROM subscriptions WHERE store_id = def_store_id) THEN
            INSERT INTO subscriptions (store_id, plan_id, status)
            VALUES (def_store_id, free_plan_id, 'active');
        END IF;
    END IF;
END $$;
