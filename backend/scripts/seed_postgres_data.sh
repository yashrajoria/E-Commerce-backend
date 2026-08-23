#!/usr/bin/env bash
set -euo pipefail

# Seeds Postgres tables (users, addresses, orders, order_items, payments,
# stripe_processed_events, coupons, notification_logs, shipments) with
# realistic demo data. Run after seed_demo_data.sh so order_items can
# reference real product IDs from DynamoDB.
#
# Demo login password for every seeded user: Demo123!
# (bcrypt hash below was generated with: htpasswd -nbBC 10 x 'Demo123!')

COMPOSE_FILES=(-f docker-compose.yml -f docker-compose.localstack.yml)
POSTGRES_DB="${POSTGRES_DB:-ecommerce}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"
GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
DEMO_PASSWORD_HASH='$2y$10$lRXXtlaZwRyVaWaNhGphOuZYVqF2/CV5gwhCSoA9L/RxFQIg/ZybW'
DEMO_EMAILS=(
  "alice.johnson@shopswift-demo.test"
  "ben.carter@shopswift-demo.test"
  "chloe.nguyen@shopswift-demo.test"
  "daniel.smith@shopswift-demo.test"
  "elena.ruiz@shopswift-demo.test"
  "merchant.owner@shopswift-demo.test"
)

# Product IDs from scripts/seed_demo_data.sh (next_product_uuid 1..37).
P1="00000000-0000-4000-8000-000000000001"   # Nova Wireless Mouse - 3999
P3="00000000-0000-4000-8000-000000000003"   # Pulse Noise-Cancel Headphones - 14999
P5="00000000-0000-4000-8000-000000000005"   # Orbit 14 Ultrabook - 99900
P9="00000000-0000-4000-8000-000000000009"   # StrideFlex Running Shoes - 11900
P12="00000000-0000-4000-8000-00000000000c"  # Luna Midi Dress - 5999
P13="00000000-0000-4000-8000-00000000000d"  # Hydra Glow Serum - 2799

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

ensure_postgres_running() {
  local status
  status="$(docker compose "${COMPOSE_FILES[@]}" ps --status=running --services | tr '\n' ' ')"
  if [[ "$status" != *"postgres"* ]]; then
    echo "Postgres is not running. Start the stack first: ./scripts/dev-up.sh" >&2
    exit 1
  fi
}

psql_exec() {
  docker compose "${COMPOSE_FILES[@]}" exec -T postgres \
    psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" </dev/null >/dev/null
}

run_sql() {
  docker compose "${COMPOSE_FILES[@]}" exec -T postgres \
    psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" "$@"
}

ensure_users_table() {
  # This script never creates `users` itself (see ensure_schema comment) — it
  # only exists once auth-service has booted and run its GORM AutoMigrate.
  # Fail with a clear next step instead of a raw "relation does not exist" error.
  local exists
  exists="$(run_sql -t -A -c "SELECT to_regclass('public.users') IS NOT NULL;" 2>/dev/null || true)"
  if [[ "$exists" != "t" ]]; then
    echo "ERROR: 'users' table does not exist yet." >&2
    echo "auth-service creates it via GORM AutoMigrate on first boot (ALLOW_AUTO_MIGRATE=true)." >&2
    echo "Start the full stack and wait for auth-service to become healthy first: ./scripts/dev-up.sh" >&2
    exit 1
  fi
}

ensure_schema() {
  # Baseline migration (000001) can't run cleanly against a users table
  # created by auth-service's GORM AutoMigrate (different column set), so
  # create only the tables this seed script needs directly, matching the
  # baseline schema. users/refresh_tokens are left untouched.
  run_sql <<'SQL'
CREATE EXTENSION IF NOT EXISTS pgcrypto;

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
  order_id uuid NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
  product_id uuid NOT NULL,
  quantity integer NOT NULL,
  price integer NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_order_items_order_id ON order_items (order_id);

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
CREATE UNIQUE INDEX IF NOT EXISTS idx_coupons_code_lower ON coupons (LOWER(code));
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
SQL
}

seed_sql() {
  run_sql <<SQL
-- Idempotent: wipe demo rows first so re-running doesn't duplicate.
DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE order_number LIKE 'DEMO-%');
DELETE FROM payments WHERE order_id IN (SELECT id FROM orders WHERE order_number LIKE 'DEMO-%');
DELETE FROM stripe_processed_events WHERE event_id LIKE 'evt_demo_%';
DELETE FROM notification_logs WHERE payload->>'seed' = 'demo';
DELETE FROM orders WHERE order_number LIKE 'DEMO-%';
DELETE FROM addresses WHERE user_id IN (SELECT id FROM users WHERE email LIKE '%@shopswift-demo.test');
DELETE FROM shipments WHERE tracking_code LIKE 'DEMO-%';
DELETE FROM coupons WHERE code IN ('WELCOME10', 'SUMMER25', 'FREESHIP', 'VIP15', 'EXPIRED5');
DELETE FROM users WHERE email LIKE '%@shopswift-demo.test';

-- Users (all share password Demo123!)
INSERT INTO users (id, email, password, name, email_verified, role, created_at, updated_at) VALUES
  ('11111111-1111-4111-8111-111111111101', 'alice.johnson@shopswift-demo.test', '$DEMO_PASSWORD_HASH', 'Alice Johnson', true, 'user', now() - interval '90 days', now() - interval '90 days'),
  ('11111111-1111-4111-8111-111111111102', 'ben.carter@shopswift-demo.test',   '$DEMO_PASSWORD_HASH', 'Ben Carter',   true, 'user', now() - interval '75 days', now() - interval '75 days'),
  ('11111111-1111-4111-8111-111111111103', 'chloe.nguyen@shopswift-demo.test','$DEMO_PASSWORD_HASH', 'Chloe Nguyen', true, 'user', now() - interval '60 days', now() - interval '60 days'),
  ('11111111-1111-4111-8111-111111111104', 'daniel.smith@shopswift-demo.test','$DEMO_PASSWORD_HASH', 'Daniel Smith', true, 'user', now() - interval '45 days', now() - interval '45 days'),
  ('11111111-1111-4111-8111-111111111105', 'elena.ruiz@shopswift-demo.test',  '$DEMO_PASSWORD_HASH', 'Elena Ruiz',   true, 'user', now() - interval '30 days', now() - interval '30 days'),
  ('11111111-1111-4111-8111-111111111199', 'merchant.owner@shopswift-demo.test', '$DEMO_PASSWORD_HASH', 'Morgan Lee (Store Owner)', true, 'admin', now() - interval '120 days', now() - interval '120 days')
ON CONFLICT (id) DO NOTHING;

-- Addresses
INSERT INTO addresses (id, user_id, type, street, city, state, postal_code, country, created_at, updated_at) VALUES
  ('22222222-2222-4222-8222-222222222201', '11111111-1111-4111-8111-111111111101', 'shipping', '482 Maple Street', 'San Francisco', 'CA', '94110', 'US', now(), now()),
  ('22222222-2222-4222-8222-222222222202', '11111111-1111-4111-8111-111111111101', 'billing',  '482 Maple Street', 'San Francisco', 'CA', '94110', 'US', now(), now()),
  ('22222222-2222-4222-8222-222222222203', '11111111-1111-4111-8111-111111111102', 'shipping', '1290 Riverside Drive', 'Austin', 'TX', '73301', 'US', now(), now()),
  ('22222222-2222-4222-8222-222222222204', '11111111-1111-4111-8111-111111111103', 'shipping', '77 Lakeshore Blvd', 'Chicago', 'IL', '60601', 'US', now(), now()),
  ('22222222-2222-4222-8222-222222222205', '11111111-1111-4111-8111-111111111104', 'shipping', '15 Highland Court', 'Seattle', 'WA', '98101', 'US', now(), now()),
  ('22222222-2222-4222-8222-222222222206', '11111111-1111-4111-8111-111111111105', 'shipping', '900 Peachtree Street', 'Atlanta', 'GA', '30309', 'US', now(), now())
ON CONFLICT (id) DO NOTHING;

-- Coupons
INSERT INTO coupons (id, code, discount_type, discount_value, min_order_value, usage_limit, used_count, expires_at, active, created_at, updated_at) VALUES
  ('33333333-3333-4333-8333-333333333301', 'WELCOME10', 'percentage', 10, 20, 500, 42, now() + interval '60 days', true, now() - interval '90 days', now()),
  ('33333333-3333-4333-8333-333333333302', 'SUMMER25',  'percentage', 25, 100, 200, 118, now() + interval '10 days', true, now() - interval '45 days', now()),
  ('33333333-3333-4333-8333-333333333303', 'FREESHIP',  'fixed', 8, 0, NULL, 300, now() + interval '30 days', true, now() - interval '30 days', now()),
  ('33333333-3333-4333-8333-333333333304', 'VIP15',     'percentage', 15, 50, 50, 12, now() + interval '20 days', true, now() - interval '15 days', now()),
  ('33333333-3333-4333-8333-333333333305', 'EXPIRED5',  'fixed', 5, 0, 100, 100, now() - interval '5 days', false, now() - interval '120 days', now() - interval '5 days')
ON CONFLICT (id) DO NOTHING;

-- Orders (amounts/prices in cents, matching seed_demo_data.sh product prices)
INSERT INTO orders (id, order_number, idempotency_key, user_id, amount, coupon_code, discount_amount, status, created_at, updated_at) VALUES
  ('44444444-4444-4444-8444-444444444401', 'DEMO-ORD-1001', 'demo-idem-1001', '11111111-1111-4111-8111-111111111101', 18998, 'WELCOME10', 2110, 'delivered',        now() - interval '40 days', now() - interval '35 days'),
  ('44444444-4444-4444-8444-444444444402', 'DEMO-ORD-1002', 'demo-idem-1002', '11111111-1111-4111-8111-111111111102', 99900, NULL,       0,    'delivered',        now() - interval '25 days', now() - interval '20 days'),
  ('44444444-4444-4444-8444-444444444403', 'DEMO-ORD-1003', 'demo-idem-1003', '11111111-1111-4111-8111-111111111103', 11900, NULL,       0,    'shipped',          now() - interval '10 days', now() - interval '8 days'),
  ('44444444-4444-4444-8444-444444444404', 'DEMO-ORD-1004', 'demo-idem-1004', '11111111-1111-4111-8111-111111111104', 8798,  'FREESHIP', 800,  'pending_payment',  now() - interval '2 days',  now() - interval '2 days'),
  ('44444444-4444-4444-8444-444444444405', 'DEMO-ORD-1005', 'demo-idem-1005', '11111111-1111-4111-8111-111111111105', 14999, NULL,       0,    'cancelled',        now() - interval '15 days', now() - interval '14 days')
ON CONFLICT (id) DO NOTHING;

-- Order items
INSERT INTO order_items (id, order_id, product_id, quantity, price) VALUES
  ('55555555-5555-4555-8555-555555555501', '44444444-4444-4444-8444-444444444401', '$P1', 1, 3999),
  ('55555555-5555-4555-8555-555555555502', '44444444-4444-4444-8444-444444444401', '$P13', 5, 2799),
  ('55555555-5555-4555-8555-555555555503', '44444444-4444-4444-8444-444444444402', '$P5', 1, 99900),
  ('55555555-5555-4555-8555-555555555504', '44444444-4444-4444-8444-444444444403', '$P9', 1, 11900),
  ('55555555-5555-4555-8555-555555555505', '44444444-4444-4444-8444-444444444404', '$P1', 1, 3999),
  ('55555555-5555-4555-8555-555555555506', '44444444-4444-4444-8444-444444444404', '$P12', 1, 5999)
    ON CONFLICT DO NOTHING;
INSERT INTO order_items (id, order_id, product_id, quantity, price) VALUES
  ('55555555-5555-4555-8555-555555555507', '44444444-4444-4444-8444-444444444405', '$P3', 1, 14999)
ON CONFLICT DO NOTHING;

-- Payments
INSERT INTO payments (payment_id, idempotency_key, order_id, user_id, amount, currency, status, stripe_payment_id, succeeded_at, created_at, updated_at) VALUES
  ('66666666-6666-4666-8666-666666666601', 'demo-idem-1001', '44444444-4444-4444-8444-444444444401', '11111111-1111-4111-8111-111111111101', 18998, 'usd', 'succeeded', 'pi_demo_1001', now() - interval '40 days', now() - interval '40 days', now() - interval '40 days'),
  ('66666666-6666-4666-8666-666666666602', 'demo-idem-1002', '44444444-4444-4444-8444-444444444402', '11111111-1111-4111-8111-111111111102', 99900, 'usd', 'succeeded', 'pi_demo_1002', now() - interval '25 days', now() - interval '25 days', now() - interval '25 days'),
  ('66666666-6666-4666-8666-666666666603', 'demo-idem-1003', '44444444-4444-4444-8444-444444444403', '11111111-1111-4111-8111-111111111103', 11900, 'usd', 'succeeded', 'pi_demo_1003', now() - interval '10 days', now() - interval '10 days', now() - interval '10 days'),
  ('66666666-6666-4666-8666-666666666604', 'demo-idem-1004', '44444444-4444-4444-8444-444444444404', '11111111-1111-4111-8111-111111111104', 8798,  'usd', 'pending',   NULL,           NULL, now() - interval '2 days', now() - interval '2 days'),
  ('66666666-6666-4666-8666-666666666605', 'demo-idem-1005', '44444444-4444-4444-8444-444444444405', '11111111-1111-4111-8111-111111111105', 14999, 'usd', 'failed',    'pi_demo_1005', NULL, now() - interval '15 days', now() - interval '14 days')
ON CONFLICT (payment_id) DO NOTHING;

-- Stripe webhook dedupe events
INSERT INTO stripe_processed_events (event_id, event_type, processed_at) VALUES
  ('evt_demo_1001', 'checkout.session.completed', now() - interval '40 days'),
  ('evt_demo_1002', 'checkout.session.completed', now() - interval '25 days'),
  ('evt_demo_1003', 'checkout.session.completed', now() - interval '10 days'),
  ('evt_demo_1005', 'checkout.session.expired',   now() - interval '14 days')
ON CONFLICT (event_id) DO NOTHING;

-- Shipments
INSERT INTO shipments (id, order_id, tracking_code, status, created_at, updated_at) VALUES
  ('77777777-7777-4777-8777-777777777701', '44444444-4444-4444-8444-444444444401', 'DEMO-TRK-1001', 'delivered', now() - interval '38 days', now() - interval '35 days'),
  ('77777777-7777-4777-8777-777777777702', '44444444-4444-4444-8444-444444444402', 'DEMO-TRK-1002', 'delivered', now() - interval '23 days', now() - interval '20 days'),
  ('77777777-7777-4777-8777-777777777703', '44444444-4444-4444-8444-444444444403', 'DEMO-TRK-1003', 'in_transit', now() - interval '9 days', now() - interval '8 days')
ON CONFLICT (id) DO NOTHING;

-- Notification logs
INSERT INTO notification_logs (id, user_id, channel, template, status, payload, created_at, updated_at) VALUES
  ('88888888-8888-4888-8888-888888888801', '11111111-1111-4111-8111-111111111101', 'email', 'order_confirmation', 'sent', '{"seed":"demo","order_id":"44444444-4444-4444-8444-444444444401"}', now() - interval '40 days', now() - interval '40 days'),
  ('88888888-8888-4888-8888-888888888802', '11111111-1111-4111-8111-111111111102', 'email', 'order_confirmation', 'sent', '{"seed":"demo","order_id":"44444444-4444-4444-8444-444444444402"}', now() - interval '25 days', now() - interval '25 days'),
  ('88888888-8888-4888-8888-888888888803', '11111111-1111-4111-8111-111111111103', 'email', 'order_shipped',      'sent', '{"seed":"demo","order_id":"44444444-4444-4444-8444-444444444403"}', now() - interval '9 days', now() - interval '9 days'),
  ('88888888-8888-4888-8888-888888888804', '11111111-1111-4111-8111-111111111105', 'email', 'payment_failed',     'failed','{"seed":"demo","order_id":"44444444-4444-4444-8444-444444444405"}', now() - interval '14 days', now() - interval '14 days')
ON CONFLICT (id) DO NOTHING;
SQL
}

seed_redis_carts() {
  # Cart state lives in Redis only (cart-service owns `cart:user:{id}`), so it
  # can't be seeded by SQL. Written here (not seed_demo_data.sh) because it
  # needs both real product IDs (from that script) and real user IDs (from
  # seed_sql above) — this is the point where both exist. TTL matches
  # cart-service's default (7 days, cart-service/config).
  echo "Seeding demo carts in Redis..."
  local now
  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  local carts=(
    "11111111-1111-4111-8111-111111111101|[{\"product_id\":\"$P1\",\"quantity\":2},{\"product_id\":\"$P13\",\"quantity\":1}]"
    "11111111-1111-4111-8111-111111111102|[{\"product_id\":\"$P5\",\"quantity\":1}]"
  )
  for entry in "${carts[@]}"; do
    local user_id="${entry%%|*}"
    local items="${entry#*|}"
    local cart_json
    cart_json="$(printf '{"user_id":"%s","items":%s,"updated_at":"%s"}' "$user_id" "$items" "$now")"
    if ! docker compose "${COMPOSE_FILES[@]}" exec -T redis \
      redis-cli -x SET "cart:user:$user_id" <<<"$cart_json" >/dev/null; then
      echo "  warning: cart seed failed for user $user_id" >&2
      continue
    fi
    docker compose "${COMPOSE_FILES[@]}" exec -T redis \
      redis-cli EXPIRE "cart:user:$user_id" 604800 >/dev/null
  done
}

seed_refresh_tokens() {
  # refresh_tokens rows need a real signed JWT (jti + HMAC signature matching
  # JWT_SECRET) to be usable, which isn't practical to fabricate in bash.
  # Logging in each demo user through the real gateway exercises the actual
  # Login code path instead, giving every demo user a live, working
  # refresh_token row so the rotation/reuse-detection path has seed coverage.
  echo "Seeding refresh_tokens via live login through api-gateway ($GATEWAY_URL)..."
  local failures=0
  for email in "${DEMO_EMAILS[@]}"; do
    if ! curl -sf -o /dev/null -X POST "$GATEWAY_URL/auth/login" \
      -H "Content-Type: application/json" \
      -d "{\"email\":\"$email\",\"password\":\"Demo123!\"}"; then
      echo "  warning: login seed failed for $email" >&2
      failures=$((failures + 1))
    fi
  done
  if [[ "$failures" -gt 0 ]]; then
    echo "  warning: $failures/${#DEMO_EMAILS[@]} refresh_token seed logins failed — is api-gateway reachable at $GATEWAY_URL?" >&2
  fi
}

print_summary() {
  run_sql -t -A -F' | ' -c "
    SELECT 'users', count(*) FROM users WHERE email LIKE '%@shopswift-demo.test'
    UNION ALL SELECT 'addresses', count(*) FROM addresses WHERE user_id IN (SELECT id FROM users WHERE email LIKE '%@shopswift-demo.test')
    UNION ALL SELECT 'orders', count(*) FROM orders WHERE order_number LIKE 'DEMO-%'
    UNION ALL SELECT 'order_items', count(*) FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE order_number LIKE 'DEMO-%')
    UNION ALL SELECT 'payments', count(*) FROM payments WHERE order_id IN (SELECT id FROM orders WHERE order_number LIKE 'DEMO-%')
    UNION ALL SELECT 'stripe_processed_events', count(*) FROM stripe_processed_events WHERE event_id LIKE 'evt_demo_%'
    UNION ALL SELECT 'coupons', count(*) FROM coupons WHERE code IN ('WELCOME10','SUMMER25','FREESHIP','VIP15','EXPIRED5')
    UNION ALL SELECT 'shipments', count(*) FROM shipments WHERE tracking_code LIKE 'DEMO-%'
    UNION ALL SELECT 'notification_logs', count(*) FROM notification_logs WHERE payload->>'seed' = 'demo'
    UNION ALL SELECT 'refresh_tokens', count(*) FROM refresh_tokens WHERE user_id IN (SELECT id FROM users WHERE email LIKE '%@shopswift-demo.test');
  "
  echo "Demo login: any *@shopswift-demo.test address, password Demo123! (merchant.owner@shopswift-demo.test is role=admin)."
}

main() {
  require_cmd docker
  require_cmd curl
  ensure_postgres_running
  ensure_users_table
  ensure_schema
  seed_sql
  seed_redis_carts
  seed_refresh_tokens
  print_summary
}

main "$@"
