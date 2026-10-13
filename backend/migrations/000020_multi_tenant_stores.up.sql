-- Migration 000020_multi_tenant_stores.up.sql
-- Multi-tenant stores, memberships, and store settings

-- 1. stores
CREATE TABLE IF NOT EXISTS stores (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        varchar(100) NOT NULL,
    slug        varchar(100) NOT NULL,
    description text,
    logo_url    text,
    currency    varchar(10) NOT NULL DEFAULT 'GBP',
    timezone    varchar(50) NOT NULL DEFAULT 'Europe/London',
    is_active   boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_stores_slug ON stores (slug) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_stores_deleted_at ON stores (deleted_at);

-- 2. store_role enum & store_memberships
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'store_role') THEN
        CREATE TYPE store_role AS ENUM ('owner', 'admin', 'manager', 'viewer');
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS store_memberships (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    store_id   uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    role       store_role NOT NULL DEFAULT 'viewer',
    invited_by uuid REFERENCES users(id),
    accepted   boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_store_memberships_user_store
    ON store_memberships (user_id, store_id);
CREATE INDEX IF NOT EXISTS idx_store_memberships_store_id ON store_memberships (store_id);

-- 3. store_settings
CREATE TABLE IF NOT EXISTS store_settings (
    store_id            uuid PRIMARY KEY REFERENCES stores(id) ON DELETE CASCADE,
    contact_email       text,
    contact_phone       text,
    address             text,
    smtp_host           text,
    smtp_port           integer,
    maintenance_mode    boolean NOT NULL DEFAULT false,
    analytics_enabled   boolean NOT NULL DEFAULT true,
    shipping_config     jsonb NOT NULL DEFAULT '{}',
    payment_config      jsonb NOT NULL DEFAULT '{}',
    updated_at          timestamptz NOT NULL DEFAULT now()
);
