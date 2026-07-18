-- Baseline schema for ShopSwift Postgres (golang-migrate).
-- Idempotent: safe to re-run (IF NOT EXISTS / OR REPLACE / exception guards).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email text NOT NULL,
  password text NOT NULL,
  name text NOT NULL,
  email_verified boolean NOT NULL DEFAULT false,
  verification_code varchar(6),
  store_name varchar(100),
  role varchar(50) NOT NULL DEFAULT 'user',
  phone_number text,
  billing_address_id uuid,
  shipping_address_id uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users (email);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_phone_number ON users (phone_number);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users (deleted_at);

CREATE TABLE IF NOT EXISTS refresh_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  token_id text NOT NULL,
  user_id uuid NOT NULL,
  revoked boolean NOT NULL DEFAULT false,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_refresh_tokens_token_id ON refresh_tokens (token_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens (user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at ON refresh_tokens (expires_at);

DO $$
BEGIN
  ALTER TABLE refresh_tokens
    ADD CONSTRAINT refresh_tokens_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE;
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS addresses (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL,
  type varchar(20) NOT NULL,
  street text NOT NULL,
  city text NOT NULL,
  state text NOT NULL,
  postal_code text NOT NULL,
  country text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_addresses_user_id ON addresses (user_id);

DO $$
BEGIN
  ALTER TABLE addresses
    ADD CONSTRAINT addresses_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE;
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
  ALTER TABLE addresses
    ADD CONSTRAINT addresses_type_check
    CHECK (type IN ('billing', 'shipping'));
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS orders (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_number text NOT NULL,
  idempotency_key varchar(128),
  user_id uuid NOT NULL,
  amount integer NOT NULL,
  coupon_code varchar(50),
  discount_amount integer NOT NULL DEFAULT 0,
  status varchar(20) NOT NULL DEFAULT 'pending_payment',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_order_number ON orders (order_number);
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_idempotency_key ON orders (idempotency_key);
CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders (user_id);

CREATE TABLE IF NOT EXISTS order_items (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id uuid NOT NULL,
  product_id uuid NOT NULL,
  quantity integer NOT NULL,
  price integer NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_order_items_order_id ON order_items (order_id);

DO $$
BEGIN
  ALTER TABLE order_items
    ADD CONSTRAINT order_items_order_id_fkey
    FOREIGN KEY (order_id) REFERENCES orders (id) ON DELETE CASCADE;
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS payments (
  payment_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  idempotency_key varchar(128),
  order_id uuid NOT NULL,
  user_id uuid NOT NULL,
  amount integer NOT NULL,
  currency varchar(10) NOT NULL,
  status varchar(20) NOT NULL,
  checkout_url varchar(1024),
  stripe_payment_id text,
  stripe_event_payload jsonb,
  succeeded_at timestamptz,
  failed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_idempotency_key ON payments (idempotency_key);
CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_stripe_payment_id ON payments (stripe_payment_id);
CREATE INDEX IF NOT EXISTS idx_payments_order_id ON payments (order_id);
CREATE INDEX IF NOT EXISTS idx_payments_user_id ON payments (user_id);

CREATE TABLE IF NOT EXISTS stripe_processed_events (
  event_id text PRIMARY KEY,
  event_type text NOT NULL,
  processed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS coupons (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code varchar(64) NOT NULL,
  discount_type varchar(32) NOT NULL,
  discount_value numeric NOT NULL,
  min_order_value numeric,
  usage_limit integer,
  used_count integer NOT NULL DEFAULT 0,
  expires_at timestamptz,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_coupons_code ON coupons (code);
CREATE UNIQUE INDEX IF NOT EXISTS idx_coupons_code_lower
  ON coupons (LOWER(code));
CREATE INDEX IF NOT EXISTS idx_coupons_active ON coupons (active, expires_at);

CREATE TABLE IF NOT EXISTS notification_logs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid,
  channel varchar(32),
  template varchar(128),
  status varchar(32),
  payload jsonb,
  error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notification_logs_user_id ON notification_logs (user_id);

-- Future / unused by shipping-service runtime:
CREATE TABLE IF NOT EXISTS shipments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id uuid,
  tracking_code text,
  status varchar(50),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_shipments_order_id ON shipments (order_id);
CREATE INDEX IF NOT EXISTS idx_shipments_tracking_code ON shipments (tracking_code);
CREATE INDEX IF NOT EXISTS idx_shipments_status ON shipments (status);
