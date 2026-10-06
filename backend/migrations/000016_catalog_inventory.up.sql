-- Stock levels and per-order reservations (replaces DynamoDB Inventory +
-- order_reservations map). Idempotent.

CREATE TABLE IF NOT EXISTS catalog.inventory (
  product_id uuid PRIMARY KEY REFERENCES catalog.products (id),
  available integer NOT NULL DEFAULT 0 CHECK (available >= 0),
  reserved integer NOT NULL DEFAULT 0 CHECK (reserved >= 0),
  threshold integer NOT NULL DEFAULT 0 CHECK (threshold >= 0),
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- One row per (order, product). status moves reserved → confirmed | released, once.
CREATE TABLE IF NOT EXISTS catalog.stock_reservations (
  order_id text NOT NULL,
  product_id uuid NOT NULL REFERENCES catalog.inventory (product_id),
  quantity integer NOT NULL CHECK (quantity > 0),
  status text NOT NULL DEFAULT 'reserved' CHECK (status IN ('reserved', 'confirmed', 'released')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (order_id, product_id)
);
CREATE INDEX IF NOT EXISTS idx_stock_reservations_product
  ON catalog.stock_reservations (product_id);
