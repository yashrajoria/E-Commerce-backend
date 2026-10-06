-- Catalog tables, owned only by catalog-service. Own schema; no FKs to other
-- services' tables. Idempotent (see migrations/README.md).

CREATE SCHEMA IF NOT EXISTS catalog;

CREATE TABLE IF NOT EXISTS catalog.categories (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  slug text NOT NULL,
  image text NOT NULL DEFAULT '',
  level integer NOT NULL DEFAULT 0 CHECK (level >= 0),
  is_active boolean NOT NULL DEFAULT true,
  -- ponytail: jsonb arrays, never queried in SQL (the service builds the tree in
  -- memory from FindAll). Normalise into a category_parents table if the tree
  -- ever needs SQL traversal.
  parent_ids jsonb NOT NULL DEFAULT '[]',
  ancestors jsonb NOT NULL DEFAULT '[]',
  path jsonb NOT NULL DEFAULT '[]',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
-- Names and slugs are intentionally NOT unique: the seed data reuses
-- "accessories", "lighting", "tools" under different parents.
CREATE INDEX IF NOT EXISTS idx_categories_name
  ON catalog.categories (name) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS catalog.products (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  sku text NOT NULL,
  price numeric(12,2) NOT NULL CHECK (price >= 0),
  quantity integer NOT NULL DEFAULT 0 CHECK (quantity >= 0),
  description text NOT NULL DEFAULT '',
  brand text NOT NULL DEFAULT '',
  images jsonb NOT NULL DEFAULT '[]',
  category_path jsonb NOT NULL DEFAULT '[]',
  is_featured boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
-- Live SKUs are unique; a soft-deleted SKU can be reused.
CREATE UNIQUE INDEX IF NOT EXISTS idx_products_sku_live
  ON catalog.products (sku) WHERE deleted_at IS NULL;
-- Default listing (newest first) and its LIMIT/OFFSET paging.
CREATE INDEX IF NOT EXISTS idx_products_created_live
  ON catalog.products (created_at DESC, id) WHERE deleted_at IS NULL;
-- brand = ? ORDER BY created_at DESC
CREATE INDEX IF NOT EXISTS idx_products_brand_created_live
  ON catalog.products (brand, created_at DESC) WHERE deleted_at IS NULL;
-- is_featured = true ORDER BY created_at DESC (tiny partial index)
CREATE INDEX IF NOT EXISTS idx_products_featured_live
  ON catalog.products (created_at DESC) WHERE is_featured AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS catalog.product_categories (
  category_id uuid NOT NULL REFERENCES catalog.categories (id),
  product_id uuid NOT NULL REFERENCES catalog.products (id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (category_id, product_id)
);
-- PK covers category → products; this covers product → categories and the FK.
CREATE INDEX IF NOT EXISTS idx_product_categories_product
  ON catalog.product_categories (product_id);
