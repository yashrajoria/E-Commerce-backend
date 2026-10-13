-- Migration 000021_store_scoping.up.sql
-- Add store_id to catalog, orders, coupons, payments, notification logs, and agent audit log

DO $$
BEGIN
    -- Products
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'catalog' AND table_name = 'products') THEN
        IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'catalog' AND table_name = 'products' AND column_name = 'store_id') THEN
            ALTER TABLE catalog.products ADD COLUMN store_id uuid REFERENCES stores(id);
        END IF;
    END IF;

    -- Categories
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'catalog' AND table_name = 'categories') THEN
        IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'catalog' AND table_name = 'categories' AND column_name = 'store_id') THEN
            ALTER TABLE catalog.categories ADD COLUMN store_id uuid REFERENCES stores(id);
        END IF;
    END IF;

    -- Orders
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'orders') THEN
        IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'orders' AND column_name = 'store_id') THEN
            ALTER TABLE orders ADD COLUMN store_id uuid REFERENCES stores(id);
        END IF;
    END IF;

    -- Coupons
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'coupons') THEN
        IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'coupons' AND column_name = 'store_id') THEN
            ALTER TABLE coupons ADD COLUMN store_id uuid REFERENCES stores(id);
        END IF;
    END IF;

    -- Notification logs
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'notification_logs') THEN
        IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'notification_logs' AND column_name = 'store_id') THEN
            ALTER TABLE notification_logs ADD COLUMN store_id uuid REFERENCES stores(id);
        END IF;
    END IF;

    -- Payments
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'payments') THEN
        IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'payments' AND column_name = 'store_id') THEN
            ALTER TABLE payments ADD COLUMN store_id uuid REFERENCES stores(id);
        END IF;
    END IF;

    -- Agent audit log
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'agent_audit_log') THEN
        IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'agent_audit_log' AND column_name = 'store_id') THEN
            ALTER TABLE agent_audit_log ADD COLUMN store_id uuid REFERENCES stores(id);
        END IF;
    END IF;
END $$;

-- Default store migration & backfill
DO $$
DECLARE
    default_store_id uuid;
    admin_user_id uuid;
BEGIN
    -- Check if default store already exists
    SELECT id INTO default_store_id FROM stores WHERE slug = 'default-store' LIMIT 1;
    IF default_store_id IS NULL THEN
        INSERT INTO stores (id, name, slug, currency)
        VALUES (gen_random_uuid(), 'Default Store', 'default-store', 'GBP')
        RETURNING id INTO default_store_id;
    END IF;

    -- Find an admin user to assign as owner
    SELECT id INTO admin_user_id FROM users WHERE role = 'admin' LIMIT 1;
    IF admin_user_id IS NOT NULL THEN
        INSERT INTO store_memberships (user_id, store_id, role, accepted)
        VALUES (admin_user_id, default_store_id, 'owner', true)
        ON CONFLICT (user_id, store_id) DO NOTHING;
    END IF;

    -- Backfill all existing records
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'catalog' AND table_name = 'products') THEN
        UPDATE catalog.products SET store_id = default_store_id WHERE store_id IS NULL;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'catalog' AND table_name = 'categories') THEN
        UPDATE catalog.categories SET store_id = default_store_id WHERE store_id IS NULL;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'orders') THEN
        UPDATE orders SET store_id = default_store_id WHERE store_id IS NULL;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'coupons') THEN
        UPDATE coupons SET store_id = default_store_id WHERE store_id IS NULL;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'notification_logs') THEN
        UPDATE notification_logs SET store_id = default_store_id WHERE store_id IS NULL;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'payments') THEN
        UPDATE payments SET store_id = default_store_id WHERE store_id IS NULL;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'agent_audit_log') THEN
        UPDATE agent_audit_log SET store_id = default_store_id WHERE store_id IS NULL;
    END IF;
END $$;

-- Indexes
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'catalog' AND table_name = 'products') THEN
        CREATE INDEX IF NOT EXISTS idx_products_store_id ON catalog.products (store_id) WHERE deleted_at IS NULL;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'catalog' AND table_name = 'categories') THEN
        CREATE INDEX IF NOT EXISTS idx_categories_store_id ON catalog.categories (store_id) WHERE deleted_at IS NULL;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'orders') THEN
        CREATE INDEX IF NOT EXISTS idx_orders_store_id ON orders (store_id);
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'coupons') THEN
        CREATE INDEX IF NOT EXISTS idx_coupons_store_id ON coupons (store_id);
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'notification_logs') THEN
        CREATE INDEX IF NOT EXISTS idx_notification_logs_store_id ON notification_logs (store_id);
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'payments') THEN
        CREATE INDEX IF NOT EXISTS idx_payments_store_id ON payments (store_id);
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'agent_audit_log') THEN
        CREATE INDEX IF NOT EXISTS idx_agent_audit_log_store_id ON agent_audit_log (store_id);
    END IF;
END $$;
