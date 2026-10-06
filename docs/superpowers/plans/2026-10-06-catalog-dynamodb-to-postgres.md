# Catalog: DynamoDB → Postgres (Supabase) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move catalog-service's four DynamoDB tables (`Products`, `Categories`, `ProductCategories`, `Inventory`) to Postgres in a dedicated `catalog` schema, then delete all DynamoDB code, infra and docs.

**Architecture:** catalog-service keeps its HTTP API and service layer unchanged. New Postgres adapters implement the *existing* repository interfaces (`repository.ProductRepo`, `repository.CategoryRepo`, `InventoryRepository` with two deliberate signature changes). Tables live in schema `catalog`, created by numbered SQL migrations (the repo's existing convention), accessed by a least-privilege role `catalog_svc` that cannot see other services' tables. Inventory reserve/release/confirm become single Postgres transactions backed by a `stock_reservations` table.

**Tech Stack:** Go 1.25, GORM (`gorm.io/gorm` v1.31.1, `gorm.io/driver/postgres` v1.6.0 — same versions as `pkg/common`), raw SQL via `db.Raw/Exec`, golang-migrate, `github.com/yashrajoria/common/db` (`ConnectPostgres`, `StartPurger`), testify. No new third-party dependencies.

## Global Constraints

- Repo root for all paths: `/Users/yashrajoria/ShopSwift/E-Commerce-backend` (git repo, default branch `main`). Do all work on branch `feat/catalog-postgres`; never commit to `main`.
- Commit messages end with `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`.
- The working tree already has ~24 modified and ~9 untracked files that belong to other in-progress work (agent-service, SQS consumers, `pkg/common/db/postgres.go` log text, CI workflows…). **Stage by explicit path only** — never `git add -A`, `git add .` or `git commit -a`. Run `git status --short` before each commit and confirm only this plan's files are staged.
- Build and test **per module** (`cd backend/services/catalog-service && go build ./... && go test ./...`). `go build ./...` from `backend/` fails ("directory prefix . does not contain main module").
- `scripts/migrate.sh` sources `backend/.env`, where `POSTGRES_HOST=postgres` only resolves inside Docker. On the host, export an explicit `DATABASE_URL=postgres://postgres:<pw>@localhost:5432/ecommerce?sslmode=disable` (read the password from `backend/.env`; never print it) before running it.
- Tooling on this machine: `migrate` is at `~/go/bin/migrate` (add to `PATH`); `psql` is not installed on the host — use `docker exec -i backend-postgres-1 psql -U postgres -d ecommerce …` for ad-hoc SQL (or `brew install libpq`). Local Postgres 16.4 runs in the `backend-postgres-1` container, port 5432 published; `schema_migrations` is at version 14, not dirty.
- Schema changes are numbered SQL migrations in `backend/migrations/` (next free numbers: `000015`, `000016`; `000008` is intentionally absent). Migrations must be idempotent per `backend/migrations/README.md` (`IF NOT EXISTS`, `DO $$ … EXCEPTION` guards). CI runs `migrate up`, then `down -all`, then `up` again, then asserts every constraint is validated — down migrations must work.
- No GORM `AutoMigrate` for catalog. SQL migrations are the only schema source (call `ConnectPostgres()` with no models).
- Postgres rules applied (Supabase best-practices): lowercase snake_case identifiers; `timestamptz`; `numeric` for money; `text` not `varchar(n)`; index every FK column; partial indexes matching `deleted_at IS NULL`; composite index equality-first; upserts via `ON CONFLICT`; consistent lock ordering; short transactions; least-privilege role; connection pool caps.
- GORM gotcha (verified): `Raw(...).Scan(&slice)` does **not** clear a slice that already has elements when the query returns zero rows. Always declare result variables fresh per query (the plan's code does; inside loops the `var` sits inside the loop body). Never hoist or reuse them.
- Service layer files `services/*_ddb.go`, controllers and routes are NOT edited, except where a task names them.
- Integration tests skip unless `TEST_DATABASE_URL` is set. They create rows with random UUIDs and delete them in `t.Cleanup`; they never `TRUNCATE`.
- Out of scope (flagged, not fixed): see "Noticed, not changing" below.

## Deliberate behavior changes

| # | Change | Why |
|---|---|---|
| 1 | `ProductRepo.FindByID` now excludes soft-deleted products (Dynamo returned them). | Fixes `GET /products/:id` returning deleted items; double-delete now 404s. |
| 2 | Live SKUs are unique (partial unique index). Soft-deleted SKUs can be reused. | Services check duplicates in app code (racy); DB now guarantees it. |
| 3 | Product lists are ordered newest first, deterministic (`created_at DESC, id`). | Dynamo `Scan` order was arbitrary; paging was unstable. |
| 4 | Products must reference existing categories (FK). Inventory rows must reference existing products (FK). | Referential integrity; services already validate categories. |
| 5 | `price >= 0`, `quantity >= 0`, `available/reserved/threshold >= 0` CHECKs. | Oversell/negative stock becomes impossible, not just unlikely. |
| 6 | Reserve replay (same order, same product, same qty) returns success; release/confirm replay returns success; release↔confirm cross-transition is an error. Release/confirm use the **stored** reservation quantity. Duplicate product lines in one request are summed. | Dynamo only deduped via `ClientRequestToken` for ~10 minutes. |
| 7 | `ListAll` uses real `LIMIT/OFFSET` ordered by `product_id` (no cursor walking). `ListAll` signature becomes `(ctx, limit, offset int)`. | Postgres has no scan cursors. Admin-only endpoint; keyset later if the table grows. |
| 8 | `SetStock` becomes one atomic upsert (`InventoryRepository.AddStock`) replacing `Get`→`Update`/`Set`. `Set` is removed from the interface. | Removes a check-then-write race. Semantics ("add to available, overwrite threshold") unchanged. |
| 9 | `ProductRepo.DeleteMany` ignores already-deleted IDs instead of failing on the first. | Idempotent; the service only checks `error`. |
| 10 | `CategoryRepo.Update` ignores keys it doesn't own (`direct_product_count`, `total_product_count`). | Those were written to Dynamo but never read back; counts are computed on read. |
| 11 | `ProductRepo` interface loses `EnsureIndexes` (migrations own indexes). | Dead with Postgres. |

## Noticed, not changing (separate follow-ups)

- `inventoryStockSync.SetStock` (main.go) calls the upsert-**add** `SetStock` on every product update that has a quantity, so repeated updates inflate stock; and `DeleteProduct`'s `SetStock(0)` adds 0, so deleted products keep sellable stock. Pre-existing; behavior is preserved by this plan.
- `CategoryServiceDDB.attachProductCounts` runs one `Count` per category (N+1). One grouped query would replace it.
- `products.quantity` duplicates `inventory.available`.

## File Structure

Create:
- `backend/migrations/000015_catalog_schema.{up,down}.sql` — schema `catalog`, `categories`, `products`, `product_categories`, indexes.
- `backend/migrations/000016_catalog_inventory.{up,down}.sql` — `inventory`, `stock_reservations`.
- `backend/infrastructure/postgres/catalog_role.sql` — `catalog_svc` least-privilege role (run once per environment, not a migration: it carries a password).
- `backend/services/catalog-service/internal/pgtest/pgtest.go` — test DB helper.
- `backend/services/catalog-service/repository/pg_util.go` — `jsonArray`, `uuidStrings`, `errRecordNotFound`.
- `backend/services/catalog-service/repository/pg_category.go` (+ `_test.go`) — `PGCategoryRepo`.
- `backend/services/catalog-service/repository/pg_product.go` (+ `_test.go`) — `PGProductRepo`, `productWhere`.
- `backend/services/catalog-service/repository/pg_role_test.go` — role isolation test.
- `backend/services/catalog-service/inventory/repository/pg_inventory_repository.go` (+ `_test.go`) — `PGInventoryRepository`.
- `backend/scripts/seed_catalog_postgres.py`, `backend/scripts/seed_catalog.sh`, `backend/scripts/seed_catalog_images.py`.

Modify:
- `backend/services/catalog-service/main.go`, `config.go`, `go.mod`, `go.sum`.
- `backend/services/catalog-service/repository/interface.go` (drop `EnsureIndexes`).
- `backend/services/catalog-service/inventory/repository/inventory_repository.go` (interface + sentinel errors only; Dynamo impl removed).
- `backend/services/catalog-service/inventory/services/inventory_service.go`, `inventory_service_test.go`.
- `backend/services/catalog-service/inventory/models/inventory.go` (drop `dynamodbav` tags).
- `.github/workflows/ci-main.yml`, `backend/docker-compose.yml`, docs and infra listed in Task 8.

Delete (Task 8): Dynamo adapters, `pkg/dynamodb`, `tools/init-dynamo`, Dynamo seeds/infra.

---

### Task 0: Pre-flight

**Files:** none changed.

- [ ] **Step 1: Branch and baseline**

```bash
cd /Users/yashrajoria/ShopSwift/E-Commerce-backend
git checkout -b feat/catalog-postgres
git status --short   # expect only the untracked backend/scripts/{dynamodb_data.json,execute_supabase_seed.py,generate_all_seeds.py,generate_seed_data.py,seed_dynamodb_full.py}
cd backend/services/catalog-service && go build ./... && go test ./...
```
Expected: build OK; catalog tests PASS (baseline recorded 2026-10-07: all green; `repository`, `services`, `inventory/services`, `cart/*` have tests, the rest report "no test files"). Branch `feat/catalog-postgres` is created off `main` at `3b9dd57`.

- [ ] **Step 2: Local Postgres with all current migrations**

```bash
cd /Users/yashrajoria/ShopSwift/E-Commerce-backend/backend
docker compose up -d postgres            # publishes 5432
./scripts/migrate.sh up
export TEST_DATABASE_URL="postgres://postgres:${POSTGRES_PASSWORD}@localhost:5432/ecommerce?sslmode=disable"
psql "$TEST_DATABASE_URL" -Atc "select count(*) from users"   # connectivity check
```
Expected: migrations apply; query returns a number.

- [ ] **Step 3: Confirm how production reaches Supabase**

Ask the user (or read the Render service env) which Supabase host/port the other services use: direct/session pooler (`:5432`) or transaction pooler (`:6543`). `commondb.ConnectPostgres` builds a plain pgx DSN, which works on `:5432`. If the answer is `:6543`, Task 5 adds `PreferSimpleProtocol` (see its Step 4 note). Also confirm `catalog` is **not** listed under Supabase Dashboard → Settings → API → "Exposed schemas" (default: only `public`, `graphql_public`).

- [ ] **Step 4: Data path (decided)**

No real AWS exists: local runs use LocalStack, and the hosted Render container runs an **in-memory** DynamoDB Local (`start.sh`) that `init-dynamo` reseeds on every start. So there is nothing to export; the plan reseeds Postgres from `backend/scripts/dynamodb_data.json`. (Task 5 Step 6's export recipe is only for the unlikely case of a persistent Dynamo table somewhere.)

---

### Task 1: Migration 000015 — catalog schema (categories, products, links)

**Files:**
- Create: `backend/migrations/000015_catalog_schema.up.sql`
- Create: `backend/migrations/000015_catalog_schema.down.sql`

**Interfaces:**
- Produces: schema `catalog`; tables `catalog.categories`, `catalog.products`, `catalog.product_categories` with the columns below. Later tasks' SQL depends on these exact names.

- [ ] **Step 1: Write the up migration**

```sql
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
```

- [ ] **Step 2: Write the down migration**

```sql
DROP SCHEMA IF EXISTS catalog CASCADE;
```

- [ ] **Step 3: Verify up, down-all, up (what CI does)**

```bash
cd /Users/yashrajoria/ShopSwift/E-Commerce-backend/backend
./scripts/migrate.sh up
psql "$TEST_DATABASE_URL" -Atc "select table_name from information_schema.tables where table_schema='catalog' order by 1"
./scripts/migrate.sh down 1 && ./scripts/migrate.sh up
psql "$TEST_DATABASE_URL" -Atc "select count(*) from pg_constraint where not convalidated"
```
Expected: lists `categories, product_categories, products`; second `up` succeeds; count `0`.

- [ ] **Step 4: Commit**

```bash
git add backend/migrations/000015_catalog_schema.*.sql
git commit -m "feat(catalog): add catalog schema migration (categories, products, links)"
```

---

### Task 2: Least-privilege role `catalog_svc` + isolation test

**Files:**
- Create: `backend/infrastructure/postgres/catalog_role.sql`
- Create: `backend/services/catalog-service/internal/pgtest/pgtest.go`
- Create: `backend/services/catalog-service/repository/pg_role_test.go`
- Modify: `.github/workflows/ci-main.yml`

**Interfaces:**
- Produces: Postgres role `catalog_svc` (LOGIN) with USAGE on schema `catalog` and CRUD on its tables, no access to `public`. `pgtest.DB(t *testing.T) *gorm.DB` used by all PG tests.

- [ ] **Step 1: Write the test helper**

`backend/services/catalog-service/internal/pgtest/pgtest.go`:

```go
// Package pgtest gives repository tests a Postgres handle, or skips the test
// when none is configured.
package pgtest

import (
	"os"
	"testing"

	commondb "github.com/yashrajoria/common/db"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DB opens TEST_DATABASE_URL, a database built by `migrate up`. Tests must
// create rows with random IDs and delete them in t.Cleanup — never TRUNCATE.
func DB(t *testing.T) *gorm.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	gdb, err := gorm.Open(postgres.Open(url), commondb.DefaultGormConfig())
	if err != nil {
		t.Fatal(err)
	}
	return gdb
}
```

Add gorm to the module (versions match `pkg/common`):

```bash
cd backend/services/catalog-service
go get gorm.io/gorm@v1.31.1 gorm.io/driver/postgres@v1.6.0
go mod tidy
go build ./...
```
Expected: builds. (`go mod tidy` keeps the dynamodb requirements for now; Task 8 drops them.)

- [ ] **Step 2: Write the failing role test**

`backend/services/catalog-service/repository/pg_role_test.go`:

```go
package repository

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	commondb "github.com/yashrajoria/common/db"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Needs the catalog_svc role from infrastructure/postgres/catalog_role.sql.
func TestPGRole_CatalogOnlySeesItsOwnSchema(t *testing.T) {
	url := os.Getenv("CATALOG_ROLE_DATABASE_URL")
	if url == "" {
		t.Skip("CATALOG_ROLE_DATABASE_URL not set")
	}
	db, err := gorm.Open(postgres.Open(url), commondb.DefaultGormConfig())
	require.NoError(t, err)

	assert.NoError(t, db.Exec("SELECT 1 FROM catalog.products LIMIT 1").Error, "own schema readable")
	assert.Error(t, db.Exec("SELECT 1 FROM public.users LIMIT 1").Error, "other services' tables denied")
	assert.Error(t, db.Exec("CREATE TABLE catalog.should_fail (id int)").Error, "no DDL")
	assert.Error(t, db.Exec("DROP TABLE catalog.products").Error, "no DROP")
}
```

- [ ] **Step 3: Write the role script**

`backend/infrastructure/postgres/catalog_role.sql`:

```sql
-- Run once per environment as an admin role (the same role that runs migrations;
-- `postgres` on Supabase), AFTER migrations 000015+:
--   psql "$ADMIN_URL" -v ON_ERROR_STOP=1 -v catalog_password=<secret> -f catalog_role.sql
-- Re-running is safe; it does not reset an existing password.

SELECT format('CREATE ROLE catalog_svc LOGIN PASSWORD %L', :'catalog_password')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'catalog_svc')
\gexec

GRANT USAGE ON SCHEMA catalog TO catalog_svc;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA catalog TO catalog_svc;
-- Covers tables created by later migrations (same admin role runs them).
ALTER DEFAULT PRIVILEGES IN SCHEMA catalog
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO catalog_svc;

REVOKE ALL ON SCHEMA catalog FROM PUBLIC;

-- Supabase: keep the Data API (PostgREST roles) out of this schema. These roles
-- do not exist on plain Postgres, hence the guard.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
    REVOKE ALL ON SCHEMA catalog FROM anon;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
    REVOKE ALL ON SCHEMA catalog FROM authenticated;
  END IF;
END $$;
```

Why no RLS: `catalog` is not an exposed API schema and `catalog_svc` is not the table owner, so enabling RLS would block the service unless a blanket policy were added, which adds no protection. Exposure is controlled by schema grants.

- [ ] **Step 4: Run the role test (skipped, then real)**

```bash
cd /Users/yashrajoria/ShopSwift/E-Commerce-backend/backend
psql "$TEST_DATABASE_URL" -v ON_ERROR_STOP=1 -v catalog_password=localpw -f infrastructure/postgres/catalog_role.sql
cd services/catalog-service
go test ./repository -run PGRole -v                                   # SKIP (env unset)
CATALOG_ROLE_DATABASE_URL="postgres://catalog_svc:localpw@localhost:5432/ecommerce?sslmode=disable" \
  go test ./repository -run PGRole -v                                 # PASS
```
Expected: SKIP then PASS. (To see it fail first, run the second command before applying the role script: connection fails.)

- [ ] **Step 5: CI — run PG tests after migrations**

In `.github/workflows/ci-main.yml`:

1. Add `backend/services/catalog-service/go.sum` to `cache-dependency-path`.
2. After the existing step `Schema drift and purge tests` (it must stay last among mutating steps; catalog tests only touch the `catalog` schema), add:

```yaml
      - name: Catalog Postgres repository tests
        env:
          TEST_DATABASE_URL: ${{ env.DB_URL }}
          CATALOG_ROLE_DATABASE_URL: postgres://catalog_svc:ci-pw@localhost:5432/ecommerce?sslmode=disable
        run: |
          psql "$DB_URL" -v ON_ERROR_STOP=1 -v catalog_password=ci-pw -f backend/infrastructure/postgres/catalog_role.sql
          (cd backend/services/catalog-service && go test -count=1 -run 'PG' ./...)
```

All DB-backed tests in this plan are named `TestPG…` so `-run PG` selects exactly them.

- [ ] **Step 6: Commit**

```bash
git add backend/infrastructure/postgres/catalog_role.sql backend/services/catalog-service/internal backend/services/catalog-service/repository/pg_role_test.go backend/services/catalog-service/go.mod backend/services/catalog-service/go.sum .github/workflows/ci-main.yml
git commit -m "feat(catalog): least-privilege catalog_svc role and PG test harness"
```

---

### Task 3: Category repository (Postgres)

**Files:**
- Create: `backend/services/catalog-service/repository/pg_util.go`
- Create: `backend/services/catalog-service/repository/pg_category.go`
- Test: `backend/services/catalog-service/repository/pg_category_test.go`

**Interfaces:**
- Consumes: `repository.CategoryRepo` (interface.go), `models.Category`, `pgtest.DB`.
- Produces: `NewPGCategoryRepo(db *gorm.DB) *PGCategoryRepo` (implements `CategoryRepo`); `jsonArray(v interface{}) string`; `uuidStrings(ids []uuid.UUID) []string`; `errRecordNotFound` (text `"record not found"` — callers match `strings.Contains(err.Error(), "not found")`).

- [ ] **Step 1: Write the failing tests**

`backend/services/catalog-service/repository/pg_category_test.go`:

```go
package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"catalog-service/internal/pgtest"
	"catalog-service/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCategory(name string) *models.Category {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &models.Category{
		ID: uuid.New(), Name: name, Slug: strings.ToLower(name), Path: []string{name},
		IsActive: true, CreatedAt: now, UpdatedAt: now,
	}
}

func TestPGCategory_CreateFindUpdateDelete(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGCategoryRepo(db)
	ctx := context.Background()

	parent := newCategory("pgt-parent-" + uuid.NewString())
	child := newCategory("pgt-child-" + uuid.NewString())
	child.ParentIDs = []uuid.UUID{parent.ID}
	child.Ancestors = []uuid.UUID{parent.ID}
	child.Level = 1
	t.Cleanup(func() {
		db.Exec("DELETE FROM catalog.categories WHERE id IN ?", []string{parent.ID.String(), child.ID.String()})
	})
	require.NoError(t, r.Create(ctx, parent))
	require.NoError(t, r.Create(ctx, child))

	got, err := r.FindByID(ctx, child.ID)
	require.NoError(t, err)
	assert.Equal(t, child.Name, got.Name)
	assert.Equal(t, []uuid.UUID{parent.ID}, got.ParentIDs)
	assert.Equal(t, []uuid.UUID{parent.ID}, got.Ancestors)
	assert.Equal(t, []string{child.Name}, got.Path)
	assert.Equal(t, 1, got.Level)
	assert.True(t, got.IsActive)

	byName, err := r.FindByName(ctx, child.Name)
	require.NoError(t, err)
	assert.Equal(t, child.ID, byName.ID)

	many, err := r.FindByNames(ctx, []string{parent.Name, child.Name, child.Name, ""})
	require.NoError(t, err)
	assert.Len(t, many, 2)

	// Unknown keys (stored counts are computed on read) are ignored; known keys apply.
	require.NoError(t, r.Update(ctx, child.ID, map[string]interface{}{
		"name": child.Name + "-x", "direct_product_count": 3, "parent_ids": []uuid.UUID{},
	}))
	got, err = r.FindByID(ctx, child.ID)
	require.NoError(t, err)
	assert.Equal(t, child.Name+"-x", got.Name)
	assert.Empty(t, got.ParentIDs)
	assert.ErrorContains(t, r.Update(ctx, uuid.New(), map[string]interface{}{"name": "x"}), "not found")

	require.NoError(t, r.Delete(ctx, child.ID))
	_, err = r.FindByID(ctx, child.ID)
	assert.ErrorContains(t, err, "not found")
	all, err := r.FindAll(ctx)
	require.NoError(t, err)
	for _, c := range all {
		assert.NotEqual(t, child.ID, c.ID)
	}
	assert.ErrorContains(t, r.Delete(ctx, child.ID), "not found")
}

func TestPGCategory_HasProducts(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGCategoryRepo(db)
	ctx := context.Background()

	c := newCategory("pgt-has-" + uuid.NewString())
	t.Cleanup(func() { db.Exec("DELETE FROM catalog.categories WHERE id = ?", c.ID) })
	require.NoError(t, r.Create(ctx, c))
	pid := uuid.New()
	t.Cleanup(func() { db.Exec("DELETE FROM catalog.products WHERE id = ?", pid) }) // runs first (LIFO)

	has, err := r.HasProducts(ctx, c.ID)
	require.NoError(t, err)
	assert.False(t, has)

	require.NoError(t, db.Exec(`INSERT INTO catalog.products (id, name, sku, price) VALUES (?, 'p', ?, 1)`,
		pid, "PGT-"+pid.String()).Error)
	require.NoError(t, db.Exec(`INSERT INTO catalog.product_categories (category_id, product_id) VALUES (?, ?)`,
		c.ID, pid).Error)

	has, err = r.HasProducts(ctx, c.ID)
	require.NoError(t, err)
	assert.True(t, has)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd backend/services/catalog-service && go test ./repository -run PGCategory -v`
Expected: FAIL to compile (`undefined: NewPGCategoryRepo`).

- [ ] **Step 3: Write `pg_util.go`**

```go
package repository

import (
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

// errRecordNotFound keeps the text callers match on (strings.Contains "not found").
var errRecordNotFound = errors.New("record not found")

// jsonArray renders a slice for a jsonb array column ("[]" for nil).
func jsonArray(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return "[]"
	}
	return string(b)
}

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}
```

- [ ] **Step 4: Write `pg_category.go`**

```go
package repository

import (
	"context"
	"encoding/json"
	"time"

	"catalog-service/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PGCategoryRepo is the Postgres-backed CategoryRepo (schema catalog).
type PGCategoryRepo struct{ db *gorm.DB }

var _ CategoryRepo = (*PGCategoryRepo)(nil)

func NewPGCategoryRepo(db *gorm.DB) *PGCategoryRepo { return &PGCategoryRepo{db: db} }

type pgCategory struct {
	ID        uuid.UUID
	Name      string
	Slug      string
	Image     string
	Level     int
	IsActive  bool
	ParentIDs []byte `gorm:"column:parent_ids"`
	Ancestors []byte
	Path      []byte
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

const categoryCols = `id, name, slug, image, level, is_active, parent_ids, ancestors, path, created_at, updated_at, deleted_at`

func (c pgCategory) model() (*models.Category, error) {
	m := &models.Category{
		ID: c.ID, Name: c.Name, Slug: c.Slug, Image: c.Image, Level: c.Level,
		IsActive: c.IsActive, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, DeletedAt: c.DeletedAt,
	}
	for _, f := range []struct {
		src []byte
		dst interface{}
	}{{c.ParentIDs, &m.ParentIDs}, {c.Ancestors, &m.Ancestors}, {c.Path, &m.Path}} {
		if err := json.Unmarshal(f.src, f.dst); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// find returns live categories matching where, ordered for stable output.
func (r *PGCategoryRepo) find(ctx context.Context, where string, args ...interface{}) ([]models.Category, error) {
	var rows []pgCategory
	q := "SELECT " + categoryCols + " FROM catalog.categories WHERE deleted_at IS NULL AND " + where + " ORDER BY level, name, id"
	if err := r.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]models.Category, 0, len(rows))
	for _, row := range rows {
		m, err := row.model()
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, nil
}

func (r *PGCategoryRepo) FindByID(ctx context.Context, id uuid.UUID) (*models.Category, error) {
	cats, err := r.find(ctx, "id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(cats) == 0 {
		return nil, errRecordNotFound
	}
	return &cats[0], nil
}

func (r *PGCategoryRepo) FindByName(ctx context.Context, name string) (*models.Category, error) {
	cats, err := r.find(ctx, "name = ?", name)
	if err != nil {
		return nil, err
	}
	if len(cats) == 0 {
		return nil, errRecordNotFound
	}
	return &cats[0], nil
}

func (r *PGCategoryRepo) FindByNames(ctx context.Context, names []string) ([]models.Category, error) {
	seen := make(map[string]struct{}, len(names))
	uniq := make([]string, 0, len(names))
	for _, n := range names {
		if _, dup := seen[n]; n == "" || dup {
			continue
		}
		seen[n] = struct{}{}
		uniq = append(uniq, n)
	}
	if len(uniq) == 0 {
		return []models.Category{}, nil
	}
	return r.find(ctx, "name IN ?", uniq)
}

func (r *PGCategoryRepo) FindAll(ctx context.Context) ([]models.Category, error) {
	return r.find(ctx, "TRUE")
}

func (r *PGCategoryRepo) Create(ctx context.Context, c *models.Category) error {
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO catalog.categories
		  (id, name, slug, image, level, is_active, parent_ids, ancestors, path, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?::jsonb, ?::jsonb, ?::jsonb, ?, ?)`,
		c.ID, c.Name, c.Slug, c.Image, c.Level, c.IsActive,
		jsonArray(c.ParentIDs), jsonArray(c.Ancestors), jsonArray(c.Path), c.CreatedAt, c.UpdatedAt,
	).Error
}

// Update applies the columns this repo owns; other keys (e.g. the stored product
// counts nothing reads back) are ignored.
func (r *PGCategoryRepo) Update(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error {
	set := map[string]interface{}{}
	for k, v := range updates {
		switch k {
		case "name", "slug", "image", "level", "is_active":
			set[k] = v
		case "parent_ids", "ancestors", "path":
			set[k] = gorm.Expr("?::jsonb", jsonArray(v))
		}
	}
	if len(set) == 0 {
		return nil
	}
	set["updated_at"] = gorm.Expr("now()")
	res := r.db.WithContext(ctx).Table("catalog.categories").
		Where("id = ? AND deleted_at IS NULL", id).Updates(set)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errRecordNotFound
	}
	return nil
}

func (r *PGCategoryRepo) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Exec(
		`UPDATE catalog.categories SET deleted_at = now(), updated_at = now() WHERE id = ? AND deleted_at IS NULL`, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errRecordNotFound
	}
	return nil
}

// HasProducts: product delete removes its links, so any link row is a live product.
func (r *PGCategoryRepo) HasProducts(ctx context.Context, categoryID uuid.UUID) (bool, error) {
	var ok bool
	err := r.db.WithContext(ctx).
		Raw(`SELECT EXISTS (SELECT 1 FROM catalog.product_categories WHERE category_id = ?)`, categoryID).
		Scan(&ok).Error
	return ok, err
}
```

- [ ] **Step 5: Run the tests**

Run: `cd backend/services/catalog-service && go vet ./repository && go test ./repository -run PGCategory -v`
Expected: PASS (both). If scanning `uuid.UUID`/`[]byte`/jsonb misbehaves, fix here — these tests are the proof that GORM maps the types correctly for Tasks 4 and 7.

- [ ] **Step 6: Commit**

```bash
git add backend/services/catalog-service/repository/pg_util.go backend/services/catalog-service/repository/pg_category.go backend/services/catalog-service/repository/pg_category_test.go
git commit -m "feat(catalog): Postgres category repository"
```

---

### Task 4: Product repository (Postgres)

**Files:**
- Create: `backend/services/catalog-service/repository/pg_product.go`
- Test: `backend/services/catalog-service/repository/pg_product_test.go`
- Modify: `backend/services/catalog-service/repository/interface.go` (remove `EnsureIndexes`)

**Interfaces:**
- Consumes: `ProductRepo`, `models.Product`, `jsonArray`, `uuidStrings`, `errRecordNotFound`, `NewPGCategoryRepo` (tests), `pgtest.DB`.
- Produces: `NewPGProductRepo(db *gorm.DB) *PGProductRepo` implementing `ProductRepo`; `productWhere(filter map[string]interface{}) (string, []interface{})`.
- Filter keys honored (unchanged contract with `product_service_ddb.go`): `is_featured` bool, `brand` string, `min_price`/`max_price` numeric, `category_ids` `[]string` or `string` (any-of), `in_stock` bool.

- [ ] **Step 1: Write the failing unit test for the filter builder**

`backend/services/catalog-service/repository/pg_product_test.go` (first part):

```go
package repository

import (
	"context"
	"testing"
	"time"

	"catalog-service/internal/pgtest"
	"catalog-service/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProductWhere(t *testing.T) {
	where, args := productWhere(nil)
	assert.Equal(t, "TRUE", where)
	assert.Empty(t, args)

	where, args = productWhere(map[string]interface{}{
		"is_featured": true, "brand": "Acme", "min_price": 5.0, "max_price": 9.0,
		"category_ids": []string{"c1", "c2"}, "in_stock": true,
	})
	assert.Equal(t, "p.is_featured = ? AND p.brand = ? AND p.price >= ? AND p.price <= ? AND "+
		"EXISTS (SELECT 1 FROM catalog.product_categories pc WHERE pc.product_id = p.id AND pc.category_id IN ?) "+
		"AND p.quantity > 0", where)
	assert.Equal(t, []interface{}{true, "Acme", 5.0, 9.0, []string{"c1", "c2"}}, args)

	where, args = productWhere(map[string]interface{}{"category_ids": "c1", "in_stock": false})
	assert.Contains(t, where, "pc.category_id IN ?")
	assert.Contains(t, where, "p.quantity <= 0")
	assert.Equal(t, []interface{}{[]string{"c1"}}, args)
}
```

- [ ] **Step 2: Write the failing integration tests**

Append to `pg_product_test.go`:

```go
func newProduct(brand string) models.Product {
	id := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	return models.Product{
		ID: id, Name: "pgt " + id.String()[:8], SKU: "PGT-" + id.String(), Price: 19.99, Quantity: 5,
		Brand: brand, Images: []string{"a.jpg", "b.jpg"}, CategoryPath: []string{"x"},
		CreatedAt: now, UpdatedAt: now,
	}
}

// makeCategory registers cleanup; call it BEFORE cleanupProducts so products are removed first (LIFO).
func makeCategory(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	c := newCategory("pgt-cat-" + uuid.NewString())
	require.NoError(t, NewPGCategoryRepo(db).Create(context.Background(), c))
	t.Cleanup(func() { db.Exec("DELETE FROM catalog.categories WHERE id = ?", c.ID) })
	return c.ID
}

func cleanupProducts(t *testing.T, db *gorm.DB, ids ...uuid.UUID) {
	t.Helper()
	t.Cleanup(func() { db.Exec("DELETE FROM catalog.products WHERE id IN ?", uuidStrings(ids)) })
}

func productIDs(ps []*models.Product) []uuid.UUID {
	out := make([]uuid.UUID, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}

func TestPGProduct_CreateAndFindByID(t *testing.T) {
	db := pgtest.DB(t)
	pr := NewPGProductRepo(db)
	ctx := context.Background()
	cat1, cat2 := makeCategory(t, db), makeCategory(t, db)

	p := newProduct("pgt-brand-" + uuid.NewString())
	p.CategoryIDs = []uuid.UUID{cat1, cat2}
	p.Description = "desc"
	cleanupProducts(t, db, p.ID)
	require.NoError(t, pr.Create(ctx, &p))

	got, err := pr.FindByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.SKU, got.SKU)
	assert.InDelta(t, 19.99, got.Price, 0.001)
	assert.Equal(t, 5, got.Quantity)
	assert.Equal(t, "desc", got.Description)
	assert.Equal(t, []string{"a.jpg", "b.jpg"}, got.Images)
	assert.Equal(t, []string{"x"}, got.CategoryPath)
	assert.ElementsMatch(t, []uuid.UUID{cat1, cat2}, got.CategoryIDs)
	assert.False(t, got.IsFeatured)
}

func TestPGProduct_FindFiltersPaginationCount(t *testing.T) {
	db := pgtest.DB(t)
	pr := NewPGProductRepo(db)
	ctx := context.Background()
	cat := makeCategory(t, db)
	brand := "pgt-brand-" + uuid.NewString()
	base := time.Now().UTC().Truncate(time.Microsecond)

	mk := func(price float64, qty int, featured bool, age time.Duration) models.Product {
		p := newProduct(brand)
		p.Price, p.Quantity, p.IsFeatured = price, qty, featured
		p.CreatedAt = base.Add(-age)
		p.CategoryIDs = []uuid.UUID{cat}
		return p
	}
	a := mk(10, 0, false, 3*time.Second) // oldest
	b := mk(20, 5, true, 2*time.Second)
	c := mk(30, 5, false, time.Second) // newest
	cleanupProducts(t, db, a.ID, b.ID, c.ID)
	require.NoError(t, pr.CreateMany(ctx, []models.Product{a, b, c}))

	find := func(filter map[string]interface{}, limit, skip int) []uuid.UUID {
		ps, err := pr.Find(ctx, filter, limit, skip)
		require.NoError(t, err)
		return productIDs(ps)
	}
	byBrand := map[string]interface{}{"brand": brand}

	assert.Equal(t, []uuid.UUID{c.ID, b.ID, a.ID}, find(byBrand, 0, 0), "newest first")
	assert.Equal(t, []uuid.UUID{c.ID, b.ID}, find(byBrand, 2, 0))
	assert.Equal(t, []uuid.UUID{a.ID}, find(byBrand, 2, 2))
	assert.Equal(t, []uuid.UUID{b.ID}, find(map[string]interface{}{"brand": brand, "is_featured": true}, 0, 0))
	assert.Equal(t, []uuid.UUID{b.ID}, find(map[string]interface{}{"brand": brand, "min_price": 15.0, "max_price": 25.0}, 0, 0))
	assert.Equal(t, []uuid.UUID{c.ID, b.ID}, find(map[string]interface{}{"brand": brand, "in_stock": true}, 0, 0))
	assert.Equal(t, []uuid.UUID{a.ID}, find(map[string]interface{}{"brand": brand, "in_stock": false}, 0, 0))
	assert.Equal(t, []uuid.UUID{c.ID, b.ID, a.ID}, find(map[string]interface{}{"category_ids": []string{cat.String()}}, 0, 0))
	assert.Equal(t, []uuid.UUID{c.ID, b.ID, a.ID}, find(map[string]interface{}{"category_ids": cat.String()}, 0, 0))

	n, err := pr.Count(ctx, byBrand)
	require.NoError(t, err)
	assert.EqualValues(t, 3, n)
	n, err = pr.Count(ctx, map[string]interface{}{"brand": brand, "in_stock": true})
	require.NoError(t, err)
	assert.EqualValues(t, 2, n)
}

func TestPGProduct_UpdateDeleteAndSKU(t *testing.T) {
	db := pgtest.DB(t)
	pr, cr := NewPGProductRepo(db), NewPGCategoryRepo(db)
	ctx := context.Background()
	cat1, cat2 := makeCategory(t, db), makeCategory(t, db)

	p := newProduct("pgt-brand-" + uuid.NewString())
	p.CategoryIDs = []uuid.UUID{cat1}
	dup := newProduct(p.Brand)
	dup.SKU = p.SKU
	cleanupProducts(t, db, p.ID, dup.ID)
	require.NoError(t, pr.Create(ctx, &p))

	// Update scalar + replace categories; unknown keys ignored; missing id errors.
	require.NoError(t, pr.Update(ctx, p.ID, map[string]interface{}{
		"price": 42.5, "category_ids": []uuid.UUID{cat2}, "images": []string{"z.jpg"}, "bogus": 1,
	}))
	got, err := pr.FindByID(ctx, p.ID)
	require.NoError(t, err)
	assert.InDelta(t, 42.5, got.Price, 0.001)
	assert.Equal(t, []uuid.UUID{cat2}, got.CategoryIDs)
	assert.Equal(t, []string{"z.jpg"}, got.Images)
	assert.ErrorContains(t, pr.Update(ctx, uuid.New(), map[string]interface{}{"price": 1.0}), "not found")

	// A live duplicate SKU is rejected by the database.
	assert.Error(t, pr.Create(ctx, &dup))

	found, err := pr.FindBySKUs(ctx, []string{p.SKU, "", p.SKU})
	require.NoError(t, err)
	assert.Len(t, found, 1)
	byIDs, err := pr.GetProductsByIDs(ctx, []string{p.ID.String(), "not-a-uuid"})
	require.NoError(t, err)
	assert.Len(t, byIDs, 1)

	// Soft delete hides it, drops its links, frees the SKU.
	require.NoError(t, pr.Delete(ctx, p.ID))
	_, err = pr.FindByID(ctx, p.ID)
	assert.ErrorContains(t, err, "not found")
	has, err := cr.HasProducts(ctx, cat2)
	require.NoError(t, err)
	assert.False(t, has)
	found, _ = pr.FindBySKUs(ctx, []string{p.SKU})
	assert.Empty(t, found)
	byIDs, _ = pr.GetProductsByIDs(ctx, []string{p.ID.String()})
	assert.Empty(t, byIDs)
	assert.ErrorContains(t, pr.Delete(ctx, p.ID), "not found")
	require.NoError(t, pr.Create(ctx, &dup), "SKU reusable after soft delete")
}

func TestPGProduct_DeleteMany(t *testing.T) {
	db := pgtest.DB(t)
	pr := NewPGProductRepo(db)
	ctx := context.Background()
	a, b := newProduct("pgt-"+uuid.NewString()), newProduct("pgt-"+uuid.NewString())
	cleanupProducts(t, db, a.ID, b.ID)
	require.NoError(t, pr.CreateMany(ctx, []models.Product{a, b}))

	require.NoError(t, pr.DeleteMany(ctx, []uuid.UUID{a.ID, b.ID, uuid.New()}), "unknown ids are ignored")
	for _, id := range []uuid.UUID{a.ID, b.ID} {
		_, err := pr.FindByID(ctx, id)
		assert.ErrorContains(t, err, "not found")
	}
	require.NoError(t, pr.DeleteMany(ctx, nil))
}
```

Added during execution (Task 4 done): `TestPGProduct_CreateManyAcrossChunks` — 450 products × 2 categories crosses both batch boundaries (200 products/insert, 500 links/insert) and asserts a duplicate-SKU row rolls back the entire batch including links.

- [ ] **Step 3: Run to verify failure**

Run: `cd backend/services/catalog-service && go test ./repository -run 'ProductWhere|PGProduct' -v`
Expected: FAIL to compile (`undefined: productWhere`, `NewPGProductRepo`).

- [ ] **Step 4: Write `pg_product.go`**

```go
package repository

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"catalog-service/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PGProductRepo is the Postgres-backed ProductRepo (schema catalog).
type PGProductRepo struct{ db *gorm.DB }

var _ ProductRepo = (*PGProductRepo)(nil)

func NewPGProductRepo(db *gorm.DB) *PGProductRepo { return &PGProductRepo{db: db} }

type pgProduct struct {
	ID           uuid.UUID
	Name         string
	SKU          string `gorm:"column:sku"`
	Price        float64
	Quantity     int
	Description  string
	Brand        string
	Images       []byte
	CategoryPath []byte
	IsFeatured   bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
	CategoryIDs  []byte `gorm:"column:category_ids"`
}

// price is cast to float8 so the driver hands back a float64, not numeric text.
const productSelect = `
SELECT p.id, p.name, p.sku, p.price::float8 AS price, p.quantity, p.description, p.brand,
       p.images, p.category_path, p.is_featured, p.created_at, p.updated_at, p.deleted_at,
       COALESCE((SELECT jsonb_agg(pc.category_id) FROM catalog.product_categories pc
                 WHERE pc.product_id = p.id), '[]'::jsonb) AS category_ids
FROM catalog.products p`

func (p pgProduct) model() (*models.Product, error) {
	m := &models.Product{
		ID: p.ID, Name: p.Name, SKU: p.SKU, Price: p.Price, Quantity: p.Quantity,
		Description: p.Description, Brand: p.Brand, IsFeatured: p.IsFeatured,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, DeletedAt: p.DeletedAt,
	}
	for _, f := range []struct {
		src []byte
		dst interface{}
	}{{p.Images, &m.Images}, {p.CategoryPath, &m.CategoryPath}, {p.CategoryIDs, &m.CategoryIDs}} {
		if err := json.Unmarshal(f.src, f.dst); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// productWhere turns the service's filter map into a WHERE fragment ("TRUE" when
// empty). Fixed check order keeps the SQL deterministic.
func productWhere(filter map[string]interface{}) (string, []interface{}) {
	var conds []string
	var args []interface{}
	if v, ok := filter["is_featured"].(bool); ok {
		conds, args = append(conds, "p.is_featured = ?"), append(args, v)
	}
	if v, ok := filter["brand"].(string); ok && v != "" {
		conds, args = append(conds, "p.brand = ?"), append(args, v)
	}
	if v, ok := filter["min_price"]; ok {
		conds, args = append(conds, "p.price >= ?"), append(args, v)
	}
	if v, ok := filter["max_price"]; ok {
		conds, args = append(conds, "p.price <= ?"), append(args, v)
	}
	var cats []string
	switch v := filter["category_ids"].(type) {
	case []string:
		cats = v
	case string:
		if v != "" {
			cats = []string{v}
		}
	}
	if len(cats) > 0 {
		conds = append(conds, "EXISTS (SELECT 1 FROM catalog.product_categories pc WHERE pc.product_id = p.id AND pc.category_id IN ?)")
		args = append(args, cats)
	}
	if v, ok := filter["in_stock"].(bool); ok {
		if v {
			conds = append(conds, "p.quantity > 0")
		} else {
			conds = append(conds, "p.quantity <= 0")
		}
	}
	if len(conds) == 0 {
		return "TRUE", nil
	}
	return strings.Join(conds, " AND "), args
}

// query returns live products matching where, newest first. tail is " LIMIT ? OFFSET ?" etc.
func (r *PGProductRepo) query(ctx context.Context, where, tail string, args ...interface{}) ([]*models.Product, error) {
	var rows []pgProduct
	q := productSelect + " WHERE p.deleted_at IS NULL AND (" + where + ") ORDER BY p.created_at DESC, p.id" + tail
	if err := r.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*models.Product, 0, len(rows))
	for _, row := range rows {
		m, err := row.model()
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *PGProductRepo) FindByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	ps, err := r.query(ctx, "p.id = ?", "", id)
	if err != nil {
		return nil, err
	}
	if len(ps) == 0 {
		return nil, errRecordNotFound
	}
	return ps[0], nil
}

// Find pages with LIMIT/OFFSET. ponytail: offset paging; switch to keyset on
// (created_at, id) if deep pages get slow.
func (r *PGProductRepo) Find(ctx context.Context, filter map[string]interface{}, limit, skip int) ([]*models.Product, error) {
	where, args := productWhere(filter)
	tail := ""
	if limit > 0 {
		tail += " LIMIT ?"
		args = append(args, limit)
	}
	if skip > 0 {
		tail += " OFFSET ?"
		args = append(args, skip)
	}
	return r.query(ctx, where, tail, args...)
}

func (r *PGProductRepo) Count(ctx context.Context, filter map[string]interface{}) (int64, error) {
	where, args := productWhere(filter)
	var n int64
	err := r.db.WithContext(ctx).
		Raw("SELECT count(*) FROM catalog.products p WHERE p.deleted_at IS NULL AND ("+where+")", args...).
		Scan(&n).Error
	return n, err
}

func (r *PGProductRepo) Create(ctx context.Context, p *models.Product) error {
	return r.CreateMany(ctx, []models.Product{*p})
}

// CreateMany inserts products and their category links in one transaction.
func (r *PGProductRepo) CreateMany(ctx context.Context, ps []models.Product) error {
	if len(ps) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := insertProducts(tx, ps); err != nil {
			return err
		}
		var links [][2]uuid.UUID
		for _, p := range ps {
			for _, c := range p.CategoryIDs {
				links = append(links, [2]uuid.UUID{c, p.ID})
			}
		}
		return insertLinks(tx, links)
	})
}

// 12 bind params per product row; chunks stay far below Postgres' 65535 limit.
const productInsertChunk = 200

func insertProducts(tx *gorm.DB, ps []models.Product) error {
	for start := 0; start < len(ps); start += productInsertChunk {
		end := min(start+productInsertChunk, len(ps))
		var sb strings.Builder
		sb.WriteString(`INSERT INTO catalog.products
			(id, name, sku, price, quantity, description, brand, images, category_path, is_featured, created_at, updated_at)
			VALUES `)
		args := make([]interface{}, 0, (end-start)*12)
		for i, p := range ps[start:end] {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString("(?,?,?,?,?,?,?,?::jsonb,?::jsonb,?,?,?)")
			args = append(args, p.ID, p.Name, p.SKU, p.Price, p.Quantity, p.Description, p.Brand,
				jsonArray(p.Images), jsonArray(p.CategoryPath), p.IsFeatured, p.CreatedAt, p.UpdatedAt)
		}
		if err := tx.Exec(sb.String(), args...).Error; err != nil {
			return err
		}
	}
	return nil
}

// insertLinks takes [category, product] pairs.
func insertLinks(tx *gorm.DB, links [][2]uuid.UUID) error {
	const chunk = 500
	for start := 0; start < len(links); start += chunk {
		end := min(start+chunk, len(links))
		var sb strings.Builder
		sb.WriteString("INSERT INTO catalog.product_categories (category_id, product_id) VALUES ")
		args := make([]interface{}, 0, (end-start)*2)
		for i, l := range links[start:end] {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString("(?,?)")
			args = append(args, l[0], l[1])
		}
		sb.WriteString(" ON CONFLICT (category_id, product_id) DO NOTHING")
		if err := tx.Exec(sb.String(), args...).Error; err != nil {
			return err
		}
	}
	return nil
}

func replaceLinks(tx *gorm.DB, productID uuid.UUID, cats []uuid.UUID) error {
	if err := tx.Exec("DELETE FROM catalog.product_categories WHERE product_id = ?", productID).Error; err != nil {
		return err
	}
	links := make([][2]uuid.UUID, len(cats))
	for i, c := range cats {
		links[i] = [2]uuid.UUID{c, productID}
	}
	return insertLinks(tx, links)
}

func categoryIDsFromUpdate(updates map[string]interface{}) ([]uuid.UUID, bool) {
	switch v := updates["category_ids"].(type) {
	case []uuid.UUID:
		return v, true
	case []string:
		out := make([]uuid.UUID, 0, len(v))
		for _, s := range v {
			if u, err := uuid.Parse(s); err == nil {
				out = append(out, u)
			}
		}
		return out, true
	}
	return nil, false
}

// Update applies whitelisted columns; category_ids replaces the links atomically.
func (r *PGProductRepo) Update(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error {
	set := map[string]interface{}{}
	for k, v := range updates {
		switch k {
		case "name", "price", "quantity", "description", "brand", "sku", "is_featured":
			set[k] = v
		case "images", "category_path":
			set[k] = gorm.Expr("?::jsonb", jsonArray(v))
		}
	}
	cats, replace := categoryIDsFromUpdate(updates)
	if len(set) == 0 && !replace {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		set["updated_at"] = gorm.Expr("now()")
		res := tx.Table("catalog.products").Where("id = ? AND deleted_at IS NULL", id).Updates(set)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errRecordNotFound
		}
		if replace {
			return replaceLinks(tx, id, cats)
		}
		return nil
	})
}

func (r *PGProductRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Exec(`UPDATE catalog.products SET deleted_at = now(), updated_at = now() WHERE id = ? AND deleted_at IS NULL`, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errRecordNotFound
		}
		return tx.Exec("DELETE FROM catalog.product_categories WHERE product_id = ?", id).Error
	})
}

func (r *PGProductRepo) DeleteMany(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	strs := uuidStrings(ids)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE catalog.products SET deleted_at = now(), updated_at = now() WHERE id IN ? AND deleted_at IS NULL`, strs).Error; err != nil {
			return err
		}
		return tx.Exec("DELETE FROM catalog.product_categories WHERE product_id IN ?", strs).Error
	})
}

func (r *PGProductRepo) FindBySKUs(ctx context.Context, skus []string) ([]models.Product, error) {
	seen := make(map[string]struct{}, len(skus))
	uniq := make([]string, 0, len(skus))
	for _, s := range skus {
		if _, dup := seen[s]; s == "" || dup {
			continue
		}
		seen[s] = struct{}{}
		uniq = append(uniq, s)
	}
	if len(uniq) == 0 {
		return nil, nil
	}
	ps, err := r.query(ctx, "p.sku IN ?", "", uniq)
	if err != nil {
		return nil, err
	}
	out := make([]models.Product, len(ps))
	for i, p := range ps {
		out[i] = *p
	}
	return out, nil
}

// GetProductsByIDs skips soft-deleted products and ids that aren't UUIDs.
func (r *PGProductRepo) GetProductsByIDs(ctx context.Context, ids []string) ([]*models.Product, error) {
	valid := make([]string, 0, len(ids))
	for _, id := range ids {
		if u, err := uuid.Parse(id); err == nil {
			valid = append(valid, u.String())
		}
	}
	if len(valid) == 0 {
		return nil, nil
	}
	return r.query(ctx, "p.id IN ?", "", valid)
}
```

- [ ] **Step 5: Remove `EnsureIndexes` from the interface**

In `repository/interface.go` delete the line `EnsureIndexes(ctx context.Context) error` from `ProductRepo`. (`DynamoAdapter` keeps its method; harmless until Task 8. `main.go` still calls it on the concrete Dynamo type until Task 5.)

- [ ] **Step 6: Run the tests**

```bash
cd backend/services/catalog-service
go vet ./... && go test ./repository -run 'ProductWhere|PGProduct|PGCategory' -v
go test ./...                              # the rest of the module still passes
```
Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/services/catalog-service/repository
git commit -m "feat(catalog): Postgres product repository with filter builder"
```

---

### Task 5: Cut products + categories over to Postgres; reseed

**Files:**
- Modify: `backend/services/catalog-service/main.go`
- Modify: `backend/docker-compose.yml` (catalog-service block, lines ~110–150)
- Create: `backend/scripts/seed_catalog_postgres.py`, `backend/scripts/seed_catalog.sh`
- Track: `backend/scripts/dynamodb_data.json` (currently untracked; it is the seed source)

**Interfaces:**
- Consumes: `NewPGProductRepo`, `NewPGCategoryRepo`, `commondb.ConnectPostgres`.
- Produces: `gdb *gorm.DB` and `sqlDB *sql.DB` in `main()` (Task 7 reuses `gdb`); a readiness endpoint that pings Postgres.

- [ ] **Step 1: Wire Postgres in `main.go`**

Add import: `commondb "github.com/yashrajoria/common/db"`.

After `LoadConfig` / Redis setup and before `// --- Repositories ---`, add:

```go
	// --- Postgres (catalog schema; SQL migrations own the schema, so no AutoMigrate) ---
	gdb, err := commondb.ConnectPostgres()
	if err != nil {
		zap.L().Fatal("Failed to connect to Postgres", zap.Error(err))
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		zap.L().Fatal("Failed to get Postgres handle", zap.Error(err))
	}
	// Small fixed pool: five services share one Supabase connection budget.
	// ponytail: constants, not config; make them env-driven if tuning is ever needed.
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
```

Replace the product/category repository construction:

```go
	productRepo := repository.NewDynamoAdapter(ddbClient, cfg.DDBTableProducts)
	productRepo.WithCategoryLinksTable(cfg.DDBTableLinks)
	if err := productRepo.EnsureIndexes(context.Background()); err != nil {
		zap.L().Warn("Failed to ensure product indexes", zap.Error(err))
	}
	categoryRepo := repository.NewDynamoCategoryAdapter(ddbClient, cfg.DDBTableCategories, cfg.DDBTableProducts).
		WithProductLinks(productRepo)
```
with:

```go
	productRepo := repository.NewPGProductRepo(gdb)
	categoryRepo := repository.NewPGCategoryRepo(gdb)
```
Leave `ddbClient` and `inventoryRepo := inventoryrepository.NewDynamoInventoryRepository(...)` as they are (inventory moves in Task 7).

Replace the static readiness handler:

```go
	r.GET("/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := sqlDB.PingContext(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
```

If Task 0 Step 3 found production on the transaction pooler (`:6543`), replace `commondb.ConnectPostgres()` with a local connect that sets `postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})`; otherwise leave as is (matches the other services).

- [ ] **Step 2: Compose env for catalog-service**

In `backend/docker-compose.yml` `catalog-service.environment`, add (mirrors notification-service's block; local dev connects as `postgres`, production overrides with `catalog_svc`):

```yaml
      - POSTGRES_HOST=postgres
      - POSTGRES_PORT=5432
      - POSTGRES_USER=${CATALOG_DB_USER:-postgres}
      - POSTGRES_PASSWORD=${CATALOG_DB_PASSWORD:-${POSTGRES_PASSWORD}}
      - POSTGRES_DB=ecommerce
      - POSTGRES_SSLMODE=disable
```
and under `depends_on` add:

```yaml
      postgres:
        condition: service_healthy
```
(Leave the `DDB_*` lines until Task 7/8.)

- [ ] **Step 3: Build and run existing tests**

```bash
cd backend/services/catalog-service && go build ./... && go vet ./... && go test ./...
```
Expected: PASS (service-layer tests don't touch Dynamo).

- [ ] **Step 4: Seed converter**

`backend/scripts/seed_catalog_postgres.py`:

```python
#!/usr/bin/env python3
"""Emit SQL that loads DynamoDB-typed JSON ({"categories":[...],"products":[...],
"product_categories":[...],"inventory":[...]}) into the catalog schema.

  python3 seed_catalog_postgres.py dynamodb_data.json | psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -1

Idempotent (ON CONFLICT DO NOTHING). Soft-deleted rows are skipped.
"""
import json
import sys


def un(v):
    (t, x), = v.items()
    if t == "N":
        return float(x) if "." in x else int(x)
    if t == "L":
        return [un(i) for i in x]
    if t == "M":
        return {k: un(i) for k, i in x.items()}
    if t == "NULL":
        return None
    return x  # S, BOOL


def q(v):
    if v is None:
        return "NULL"
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, (int, float)):
        return str(v)
    return "'" + str(v).replace("'", "''") + "'"


def jb(v):
    return q(json.dumps(v)) + "::jsonb"


def rows(name):
    return [{k: un(v) for k, v in item.items()} for item in data.get(name, [])]


data = json.load(open(sys.argv[1] if len(sys.argv) > 1 else "dynamodb_data.json"))
cats = [c for c in rows("categories") if "deleted_at" not in c]
prods = [p for p in rows("products") if "deleted_at" not in p]
live_prods = {p["id"] for p in prods}
live_cats = {c["id"] for c in cats}
links = [l for l in rows("product_categories")
         if l["product_id"] in live_prods and l["category_id"] in live_cats]

print("-- categories")
for c in cats:
    print("INSERT INTO catalog.categories (id, name, slug, image, level, is_active, parent_ids, ancestors, path, created_at, updated_at) VALUES "
          f"({q(c['id'])}, {q(c['name'])}, {q(c['slug'])}, {q(c.get('image', ''))}, {c.get('level', 0)}, {q(c.get('is_active', True))}, "
          f"{jb(c.get('parent_ids', []))}, {jb(c.get('ancestors', []))}, {jb(c.get('path', []))}, "
          f"{q(c['created_at'])}, {q(c['updated_at'])}) ON CONFLICT (id) DO NOTHING;")

print("-- products")
for p in prods:
    featured = str(p.get("is_featured", "false")).lower() == "true"
    print("INSERT INTO catalog.products (id, name, sku, price, quantity, description, brand, images, category_path, is_featured, created_at, updated_at) VALUES "
          f"({q(p['id'])}, {q(p['name'])}, {q(p['sku'])}, {p['price']}, {p.get('quantity', 0)}, {q(p.get('description', ''))}, {q(p.get('brand', ''))}, "
          f"{jb(p.get('images', []))}, {jb(p.get('category_path', []))}, {q(featured)}, "
          f"{q(p['created_at'])}, {q(p['updated_at'])}) ON CONFLICT (id) DO NOTHING;")

print("-- product_categories")
for l in links:
    print(f"INSERT INTO catalog.product_categories (category_id, product_id) VALUES ({q(l['category_id'])}, {q(l['product_id'])}) ON CONFLICT DO NOTHING;")
```

`backend/scripts/seed_catalog.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
# Seed catalog tables in Postgres from dynamodb_data.json (DynamoDB-typed JSON; the
# filename is historical). Needs the catalog migrations applied.
#   DATABASE_URL=postgres://postgres:...@localhost:5432/ecommerce?sslmode=disable ./scripts/seed_catalog.sh
# Demo images (LocalStack S3 only): python3 scripts/seed_catalog_images.py
cd "$(dirname "$0")"
: "${DATABASE_URL:?set DATABASE_URL}"
python3 seed_catalog_postgres.py dynamodb_data.json | psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -1
psql "$DATABASE_URL" -Atc "select 'categories', count(*) from catalog.categories union all select 'products', count(*) from catalog.products union all select 'links', count(*) from catalog.product_categories"
```

Run it:

```bash
chmod +x backend/scripts/seed_catalog.sh
cd backend && DATABASE_URL="$TEST_DATABASE_URL" ./scripts/seed_catalog.sh
```
Expected: `categories|110`, `products|125`, `links|250` (plus any rows from earlier test runs: tests clean up after themselves, so exactly these).

- [ ] **Step 5: Smoke test the running service**

```bash
cd backend
docker compose -f docker-compose.yml -f docker-compose.localstack.yml -f docker-compose.dev-ports.yml up -d --build postgres redis localstack localstack-init catalog-service
curl -s localhost:8082/health/ready
curl -s "localhost:8082/products?limit=2" | head -c 600
curl -s "localhost:8082/categories" | head -c 400
```
Expected: `{"status":"ready"}`; two products with `category_ids`; a category tree. Inventory endpoints are still on Dynamo here, so skip them.

- [ ] **Step 6 (only if real data exists in AWS DynamoDB): export recipe**

```bash
for t in Categories Products ProductCategories Inventory; do aws dynamodb scan --table-name $t --output json | jq .Items > /tmp/$t.json; done
jq -n --slurpfile c /tmp/Categories.json --slurpfile p /tmp/Products.json \
      --slurpfile l /tmp/ProductCategories.json --slurpfile i /tmp/Inventory.json \
      '{categories:$c[0],products:$p[0],product_categories:$l[0],inventory:$i[0]}' > /tmp/real_data.json
python3 backend/scripts/seed_catalog_postgres.py /tmp/real_data.json | psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -1
```
Check counts against `aws dynamodb scan --select COUNT` per table. Skip if reseeding.

- [ ] **Step 7: Commit**

```bash
git add backend/services/catalog-service/main.go backend/docker-compose.yml backend/scripts/seed_catalog_postgres.py backend/scripts/seed_catalog.sh backend/scripts/dynamodb_data.json
git commit -m "feat(catalog): serve products and categories from Postgres"
```

---

### Task 6: Migration 000016 — inventory and reservations

**Files:**
- Create: `backend/migrations/000016_catalog_inventory.up.sql`
- Create: `backend/migrations/000016_catalog_inventory.down.sql`

**Interfaces:**
- Produces: `catalog.inventory(product_id PK, available, reserved, threshold, updated_at)`; `catalog.stock_reservations(order_id, product_id, quantity, status, created_at, updated_at; PK (order_id, product_id))`. Task 7's SQL depends on these names.

- [ ] **Step 1: Write the up migration**

```sql
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
```

- [ ] **Step 2: Write the down migration**

```sql
DROP TABLE IF EXISTS catalog.stock_reservations;
DROP TABLE IF EXISTS catalog.inventory;
```

- [ ] **Step 3: Verify and grant**

```bash
cd backend
./scripts/migrate.sh up
./scripts/migrate.sh down 1 && ./scripts/migrate.sh up
psql "$TEST_DATABASE_URL" -Atc "select count(*) from pg_constraint where not convalidated"   # 0
psql "$TEST_DATABASE_URL" -v ON_ERROR_STOP=1 -v catalog_password=localpw -f infrastructure/postgres/catalog_role.sql   # re-run: covers the new tables
```
Expected: `0`; role script re-runs cleanly. (Production: re-run the role script after this migration too, unless `ALTER DEFAULT PRIVILEGES` already covered it — it does when the same admin role ran the migration.)

- [ ] **Step 4: Commit**

```bash
git add backend/migrations/000016_catalog_inventory.*.sql
git commit -m "feat(catalog): add inventory and stock_reservations migration"
```

---

### Task 7: Inventory repository (Postgres), service changes, cutover

This task is atomic: changing the `InventoryRepository` interface breaks the Dynamo implementation, so repo, service, mock and `main.go` change together.

**Files:**
- Modify: `backend/services/catalog-service/inventory/repository/inventory_repository.go` (replace entire file: sentinel errors + interface)
- Create: `backend/services/catalog-service/inventory/repository/pg_inventory_repository.go`
- Test: `backend/services/catalog-service/inventory/repository/pg_inventory_repository_test.go`
- Modify: `backend/services/catalog-service/inventory/services/inventory_service.go`, `inventory_service_test.go`
- Modify: `backend/services/catalog-service/inventory/models/inventory.go`
- Modify: `backend/services/catalog-service/main.go`
- Modify: `backend/scripts/seed_catalog_postgres.py`

**Interfaces:**
- Produces (`InventoryRepository`):

```go
type InventoryRepository interface {
	Get(ctx context.Context, productID string) (*models.Inventory, error)
	BatchGet(ctx context.Context, productIDs []string) (map[string]*models.Inventory, error)
	AddStock(ctx context.Context, productID string, delta, threshold int) (*models.Inventory, error)
	Update(ctx context.Context, productID string, updates map[string]interface{}) error
	ReserveAll(ctx context.Context, orderID string, items []models.ReserveItem) error
	ReleaseAll(ctx context.Context, orderID string, items []models.ReserveItem) error
	ConfirmAll(ctx context.Context, orderID string, items []models.ReserveItem) error
	RestockAll(ctx context.Context, orderID string, items []models.ReserveItem) error
	CheckStock(ctx context.Context, productID string, quantity int) (*models.StockCheckResult, error)
	ListAll(ctx context.Context, limit, offset int) ([]models.Inventory, error)
}
```
- Sentinel errors: `ErrNotFound`, `ErrInsufficientStock`, `ErrDuplicateReservation`, `ErrNoReservation`. `NewPGInventoryRepository(db *gorm.DB) *PGInventoryRepository`.

- [ ] **Step 1: Confirm nothing else uses `Set` or Dynamo types in the inventory package**

```bash
cd backend/services/catalog-service
grep -rn "repo\.Set(\|\.Set(ctx" --include='*.go' inventory
grep -rn "dynamodb\|types\.AttributeValue" --include='*.go' inventory | grep -v "inventory/repository/inventory_repository.go"
```
Expected: only `inventory_service.go` (import of `types`, `ListAllStock`) and `inventory_service_test.go` (mock import + `ListAll`). Anything else → handle it here.

- [ ] **Step 2: Write the failing unit test for `mergeItems` and the integration tests**

`backend/services/catalog-service/inventory/repository/pg_inventory_repository_test.go`:

```go
package repository

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"catalog-service/internal/pgtest"
	"catalog-service/inventory/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMergeItems_SumsDuplicatesAndSortsByID(t *testing.T) {
	a, b := "00000000-0000-4000-8000-00000000000a", "00000000-0000-4000-8000-00000000000b"
	lines, err := mergeItems([]models.ReserveItem{{ProductID: b, Quantity: 1}, {ProductID: a, Quantity: 2}, {ProductID: b, Quantity: 4}})
	require.NoError(t, err)
	require.Len(t, lines, 2)
	assert.Equal(t, a, lines[0].id.String())
	assert.Equal(t, 2, lines[0].qty)
	assert.Equal(t, 5, lines[1].qty)

	_, err = mergeItems([]models.ReserveItem{{ProductID: "nope", Quantity: 1}})
	assert.Error(t, err)
}

// seedStock creates a product + inventory row and registers cleanup.
func seedStock(t *testing.T, db *gorm.DB, r *PGInventoryRepository, available int) string {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO catalog.products (id, name, sku, price) VALUES (?, 'pgt-inv', ?, 1)`,
		id, "PGT-"+id.String()).Error)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM catalog.stock_reservations WHERE product_id = ?`, id)
		db.Exec(`DELETE FROM catalog.inventory WHERE product_id = ?`, id)
		db.Exec(`DELETE FROM catalog.products WHERE id = ?`, id)
	})
	_, err := r.AddStock(context.Background(), id.String(), available, 1)
	require.NoError(t, err)
	return id.String()
}

func levels(t *testing.T, r *PGInventoryRepository, id string) (available, reserved int) {
	t.Helper()
	inv, err := r.Get(context.Background(), id)
	require.NoError(t, err)
	return inv.Available, inv.Reserved
}

func TestPGInventory_AddStockUpserts(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGInventoryRepository(db)
	id := seedStock(t, db, r, 10)

	inv, err := r.AddStock(context.Background(), id, 5, 3)
	require.NoError(t, err)
	assert.Equal(t, 15, inv.Available)
	assert.Equal(t, 3, inv.Threshold)
	assert.Equal(t, 0, inv.Reserved)

	_, err = r.Get(context.Background(), "not-a-uuid")
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = r.Get(context.Background(), uuid.NewString())
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPGInventory_ReserveConfirmLifecycleIsIdempotent(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGInventoryRepository(db)
	ctx := context.Background()
	id := seedStock(t, db, r, 10)
	items := []models.ReserveItem{{ProductID: id, Quantity: 3}}
	order := "order-" + uuid.NewString()

	require.NoError(t, r.ReserveAll(ctx, order, items))
	a, rs := levels(t, r, id)
	assert.Equal(t, []int{7, 3}, []int{a, rs})
	inv, _ := r.Get(ctx, id)
	assert.Equal(t, map[string]int{order: 3}, inv.OrderReservations)

	require.NoError(t, r.ReserveAll(ctx, order, items), "replay is a no-op")
	a, rs = levels(t, r, id)
	assert.Equal(t, []int{7, 3}, []int{a, rs})

	require.NoError(t, r.ConfirmAll(ctx, order, items))
	a, rs = levels(t, r, id)
	assert.Equal(t, []int{7, 0}, []int{a, rs})
	inv, _ = r.Get(ctx, id)
	assert.Empty(t, inv.OrderReservations)

	require.NoError(t, r.ConfirmAll(ctx, order, items), "confirm replay is a no-op")
	assert.ErrorIs(t, r.ReleaseAll(ctx, order, items), ErrNoReservation, "cannot release a confirmed reservation")
}

func TestPGInventory_ReleaseRestoresAvailable(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGInventoryRepository(db)
	ctx := context.Background()
	id := seedStock(t, db, r, 10)
	items := []models.ReserveItem{{ProductID: id, Quantity: 4}}
	order := "order-" + uuid.NewString()

	require.NoError(t, r.ReserveAll(ctx, order, items))
	require.NoError(t, r.ReleaseAll(ctx, order, items))
	a, rs := levels(t, r, id)
	assert.Equal(t, []int{10, 0}, []int{a, rs})
	require.NoError(t, r.ReleaseAll(ctx, order, items), "release replay is a no-op")
	assert.ErrorIs(t, r.ConfirmAll(ctx, order, items), ErrNoReservation, "cannot confirm a released reservation")
	assert.ErrorIs(t, r.ReleaseAll(ctx, "order-"+uuid.NewString(), items), ErrNoReservation, "never reserved")
}

func TestPGInventory_ReserveRejectsInsufficientAndConflicts(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGInventoryRepository(db)
	ctx := context.Background()
	a := seedStock(t, db, r, 10)
	b := seedStock(t, db, r, 1)
	order := "order-" + uuid.NewString()

	// Second line is short: the whole transaction rolls back, line 1 untouched.
	err := r.ReserveAll(ctx, order, []models.ReserveItem{{ProductID: a, Quantity: 1}, {ProductID: b, Quantity: 2}})
	assert.ErrorIs(t, err, ErrInsufficientStock)
	av, rs := levels(t, r, a)
	assert.Equal(t, []int{10, 0}, []int{av, rs})

	// Unknown product behaves like zero stock.
	err = r.ReserveAll(ctx, order, []models.ReserveItem{{ProductID: uuid.NewString(), Quantity: 1}})
	assert.ErrorIs(t, err, ErrInsufficientStock)

	// Same order, same product, different quantity is a conflict, not a replay.
	require.NoError(t, r.ReserveAll(ctx, order, []models.ReserveItem{{ProductID: a, Quantity: 2}}))
	err = r.ReserveAll(ctx, order, []models.ReserveItem{{ProductID: a, Quantity: 3}})
	assert.ErrorIs(t, err, ErrDuplicateReservation)
}

func TestPGInventory_RestockBatchGetCheckUpdateList(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGInventoryRepository(db)
	ctx := context.Background()
	id := seedStock(t, db, r, 5)

	require.NoError(t, r.RestockAll(ctx, "o", []models.ReserveItem{{ProductID: id, Quantity: 2}}))
	av, _ := levels(t, r, id)
	assert.Equal(t, 7, av)
	assert.ErrorIs(t, r.RestockAll(ctx, "o", []models.ReserveItem{{ProductID: uuid.NewString(), Quantity: 1}}), ErrNotFound)

	m, err := r.BatchGet(ctx, []string{id, id, uuid.NewString(), "bad"})
	require.NoError(t, err)
	require.Len(t, m, 1)
	assert.Equal(t, 7, m[id].Available)

	res, err := r.CheckStock(ctx, id, 8)
	require.NoError(t, err)
	assert.False(t, res.IsSufficient)
	res, err = r.CheckStock(ctx, uuid.NewString(), 1)
	require.NoError(t, err)
	assert.False(t, res.IsSufficient)

	require.NoError(t, r.Update(ctx, id, map[string]interface{}{"available": 20, "threshold": 4, "updated_at": "ignored"}))
	inv, _ := r.Get(ctx, id)
	assert.Equal(t, 20, inv.Available)
	assert.Equal(t, 4, inv.Threshold)
	assert.ErrorIs(t, r.Update(ctx, uuid.NewString(), map[string]interface{}{"available": 1}), ErrNotFound)

	page, err := r.ListAll(ctx, 1, 0)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(page), 1)
}

func TestPGInventory_ConcurrentReservesNeverOversell(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGInventoryRepository(db)
	ctx := context.Background()
	id := seedStock(t, db, r, 10)

	var ok, short int32
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := r.ReserveAll(ctx, "order-"+uuid.NewString(), []models.ReserveItem{{ProductID: id, Quantity: 1}})
			switch {
			case err == nil:
				atomic.AddInt32(&ok, 1)
			case assert.ErrorIs(t, err, ErrInsufficientStock):
				atomic.AddInt32(&short, 1)
			}
		}(i)
	}
	wg.Wait()
	assert.EqualValues(t, 10, ok)
	assert.EqualValues(t, 15, short)
	av, rs := levels(t, r, id)
	assert.Equal(t, []int{0, 10}, []int{av, rs})
}

func TestPGInventory_OppositeOrderMultiProductReservesDoNotDeadlock(t *testing.T) {
	db := pgtest.DB(t)
	r := NewPGInventoryRepository(db)
	ctx := context.Background()
	a, b := seedStock(t, db, r, 1000), seedStock(t, db, r, 1000)

	var wg sync.WaitGroup
	for _, items := range [][]models.ReserveItem{
		{{ProductID: a, Quantity: 1}, {ProductID: b, Quantity: 1}},
		{{ProductID: b, Quantity: 1}, {ProductID: a, Quantity: 1}},
	} {
		wg.Add(1)
		go func(items []models.ReserveItem) {
			defer wg.Done()
			for i := 0; i < 30; i++ {
				assert.NoError(t, r.ReserveAll(ctx, "order-"+uuid.NewString(), items))
			}
		}(items)
	}
	wg.Wait()
}
```

- [ ] **Step 3: Run to verify failure**

Run: `cd backend/services/catalog-service && go test ./inventory/repository -run 'MergeItems|PGInventory' -v`
Expected: FAIL to compile (`undefined: mergeItems`, `NewPGInventoryRepository`, `AddStock`).

- [ ] **Step 4: Replace `inventory_repository.go` with the interface and errors**

```go
package repository

import (
	"context"
	"errors"

	"catalog-service/inventory/models"
)

var (
	ErrNotFound             = errors.New("inventory record not found")
	ErrInsufficientStock    = errors.New("insufficient stock")
	ErrDuplicateReservation = errors.New("order already holds a different reservation for this product")
	ErrNoReservation        = errors.New("no active reservation for this order and product")
)

// InventoryRepository defines the interface for inventory data access.
type InventoryRepository interface {
	Get(ctx context.Context, productID string) (*models.Inventory, error)
	BatchGet(ctx context.Context, productIDs []string) (map[string]*models.Inventory, error)
	// AddStock upserts: adds delta to available and overwrites threshold.
	AddStock(ctx context.Context, productID string, delta, threshold int) (*models.Inventory, error)
	Update(ctx context.Context, productID string, updates map[string]interface{}) error
	ReserveAll(ctx context.Context, orderID string, items []models.ReserveItem) error
	ReleaseAll(ctx context.Context, orderID string, items []models.ReserveItem) error
	ConfirmAll(ctx context.Context, orderID string, items []models.ReserveItem) error
	RestockAll(ctx context.Context, orderID string, items []models.ReserveItem) error
	CheckStock(ctx context.Context, productID string, quantity int) (*models.StockCheckResult, error)
	ListAll(ctx context.Context, limit, offset int) ([]models.Inventory, error)
}
```

- [ ] **Step 5: Write `pg_inventory_repository.go`**

```go
package repository

import (
	"context"
	"fmt"
	"sort"
	"time"

	"catalog-service/inventory/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PGInventoryRepository implements InventoryRepository on Postgres (schema catalog).
// Reserve/release/confirm each run in one short transaction; products are always
// locked in id order so concurrent multi-product orders cannot deadlock.
type PGInventoryRepository struct{ db *gorm.DB }

var _ InventoryRepository = (*PGInventoryRepository)(nil)

func NewPGInventoryRepository(db *gorm.DB) *PGInventoryRepository {
	return &PGInventoryRepository{db: db}
}

type pgInventory struct {
	ProductID uuid.UUID
	Available int
	Reserved  int
	Threshold int
	UpdatedAt time.Time
}

const inventoryCols = "product_id, available, reserved, threshold, updated_at"

func (p pgInventory) model() *models.Inventory {
	return &models.Inventory{
		ProductID: p.ProductID.String(), Available: p.Available, Reserved: p.Reserved,
		Threshold: p.Threshold, UpdatedAt: p.UpdatedAt,
	}
}

// parseProductID maps a non-UUID id to ErrNotFound (Dynamo treated any unknown string as missing).
func parseProductID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, ErrNotFound
	}
	return id, nil
}

type stockLine struct {
	id  uuid.UUID
	qty int
}

// mergeItems sums duplicate products and sorts by id, giving every transaction the same lock order.
func mergeItems(items []models.ReserveItem) ([]stockLine, error) {
	sum := make(map[uuid.UUID]int, len(items))
	for _, it := range items {
		id, err := uuid.Parse(it.ProductID)
		if err != nil {
			return nil, fmt.Errorf("invalid product_id %q", it.ProductID)
		}
		sum[id] += it.Quantity
	}
	lines := make([]stockLine, 0, len(sum))
	for id, q := range sum {
		lines = append(lines, stockLine{id, q})
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].id.String() < lines[j].id.String() })
	return lines, nil
}

func (r *PGInventoryRepository) Get(ctx context.Context, productID string) (*models.Inventory, error) {
	id, err := parseProductID(productID)
	if err != nil {
		return nil, err
	}
	var rows []pgInventory
	if err := r.db.WithContext(ctx).
		Raw("SELECT "+inventoryCols+" FROM catalog.inventory WHERE product_id = ?", id).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	inv := rows[0].model()

	var res []struct {
		OrderID  string
		Quantity int
	}
	if err := r.db.WithContext(ctx).
		Raw(`SELECT order_id, quantity FROM catalog.stock_reservations WHERE product_id = ? AND status = 'reserved'`, id).
		Scan(&res).Error; err != nil {
		return nil, err
	}
	if len(res) > 0 {
		inv.OrderReservations = make(map[string]int, len(res))
		for _, x := range res {
			inv.OrderReservations[x.OrderID] = x.Quantity
		}
	}
	return inv, nil
}

// BatchGet omits unknown ids and does not load order reservations.
func (r *PGInventoryRepository) BatchGet(ctx context.Context, productIDs []string) (map[string]*models.Inventory, error) {
	out := make(map[string]*models.Inventory, len(productIDs))
	var ids []string
	for _, s := range productIDs {
		if id, err := uuid.Parse(s); err == nil {
			ids = append(ids, id.String())
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []pgInventory
	if err := r.db.WithContext(ctx).
		Raw("SELECT "+inventoryCols+" FROM catalog.inventory WHERE product_id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ProductID.String()] = row.model()
	}
	return out, nil
}

func (r *PGInventoryRepository) AddStock(ctx context.Context, productID string, delta, threshold int) (*models.Inventory, error) {
	id, err := uuid.Parse(productID)
	if err != nil {
		return nil, fmt.Errorf("invalid product_id %q", productID)
	}
	var rows []pgInventory
	err = r.db.WithContext(ctx).Raw(`
		INSERT INTO catalog.inventory (product_id, available, threshold, updated_at)
		VALUES (?, ?, ?, now())
		ON CONFLICT (product_id) DO UPDATE
		SET available = catalog.inventory.available + EXCLUDED.available,
		    threshold = EXCLUDED.threshold,
		    updated_at = now()
		RETURNING `+inventoryCols, id, delta, threshold).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("upsert returned no row for product %s", productID)
	}
	return rows[0].model(), nil
}

// Update sets available and/or threshold; other keys are ignored.
func (r *PGInventoryRepository) Update(ctx context.Context, productID string, updates map[string]interface{}) error {
	id, err := parseProductID(productID)
	if err != nil {
		return err
	}
	set := map[string]interface{}{}
	for _, k := range []string{"available", "threshold"} {
		if v, ok := updates[k]; ok {
			set[k] = v
		}
	}
	if len(set) == 0 {
		return nil
	}
	set["updated_at"] = gorm.Expr("now()")
	res := r.db.WithContext(ctx).Table("catalog.inventory").Where("product_id = ?", id).Updates(set)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ReserveAll reserves every line atomically. A retry of an already-applied reserve
// (same order, product, quantity) succeeds without changing stock.
func (r *PGInventoryRepository) ReserveAll(ctx context.Context, orderID string, items []models.ReserveItem) error {
	lines, err := mergeItems(items)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, l := range lines {
			// Only inserts when the product has an inventory row; a conflict means this order already reserved it.
			res := tx.Exec(`
				INSERT INTO catalog.stock_reservations (order_id, product_id, quantity)
				SELECT ?, product_id, ? FROM catalog.inventory WHERE product_id = ?
				ON CONFLICT (order_id, product_id) DO NOTHING`, orderID, l.qty, l.id)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				var prev []struct {
					Status   string
					Quantity int
				}
				if err := tx.Raw(`SELECT status, quantity FROM catalog.stock_reservations WHERE order_id = ? AND product_id = ?`,
					orderID, l.id).Scan(&prev).Error; err != nil {
					return err
				}
				switch {
				case len(prev) == 0:
					return fmt.Errorf("%w: product=%s has no inventory", ErrInsufficientStock, l.id)
				case prev[0].Status == "reserved" && prev[0].Quantity == l.qty:
					continue // retry of a reserve that already succeeded
				default:
					return fmt.Errorf("%w: order=%s product=%s", ErrDuplicateReservation, orderID, l.id)
				}
			}
			res = tx.Exec(`
				UPDATE catalog.inventory
				SET available = available - ?, reserved = reserved + ?, updated_at = now()
				WHERE product_id = ? AND available >= ?`, l.qty, l.qty, l.id, l.qty)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return fmt.Errorf("%w: product=%s", ErrInsufficientStock, l.id)
			}
		}
		return nil
	})
}

// ReleaseAll returns reserved units to available.
func (r *PGInventoryRepository) ReleaseAll(ctx context.Context, orderID string, items []models.ReserveItem) error {
	return r.settle(ctx, orderID, items, "released", true)
}

// ConfirmAll makes reserved units permanent (they stay out of available, leave reserved).
func (r *PGInventoryRepository) ConfirmAll(ctx context.Context, orderID string, items []models.ReserveItem) error {
	return r.settle(ctx, orderID, items, "confirmed", false)
}

// settle moves each reservation out of 'reserved' using the STORED quantity. Replaying
// the same transition is a no-op; any other state is ErrNoReservation.
func (r *PGInventoryRepository) settle(ctx context.Context, orderID string, items []models.ReserveItem, to string, restock bool) error {
	lines, err := mergeItems(items)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, l := range lines {
			var done []struct{ Quantity int }
			if err := tx.Raw(`
				UPDATE catalog.stock_reservations SET status = ?, updated_at = now()
				WHERE order_id = ? AND product_id = ? AND status = 'reserved'
				RETURNING quantity`, to, orderID, l.id).Scan(&done).Error; err != nil {
				return err
			}
			if len(done) == 0 {
				var cur []struct{ Status string }
				if err := tx.Raw(`SELECT status FROM catalog.stock_reservations WHERE order_id = ? AND product_id = ?`,
					orderID, l.id).Scan(&cur).Error; err != nil {
					return err
				}
				if len(cur) == 1 && cur[0].Status == to {
					continue // replay
				}
				return fmt.Errorf("%w: order=%s product=%s", ErrNoReservation, orderID, l.id)
			}
			q, back := done[0].Quantity, 0
			if restock {
				back = q
			}
			if err := tx.Exec(`
				UPDATE catalog.inventory
				SET available = available + ?, reserved = reserved - ?, updated_at = now()
				WHERE product_id = ?`, back, q, l.id).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// RestockAll adds confirmed quantities back to available (paid order cancelled).
// Idempotency is guarded upstream by the order's paid → cancelled transition.
func (r *PGInventoryRepository) RestockAll(ctx context.Context, orderID string, items []models.ReserveItem) error {
	lines, err := mergeItems(items)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, l := range lines {
			res := tx.Exec(`UPDATE catalog.inventory SET available = available + ?, updated_at = now() WHERE product_id = ?`, l.qty, l.id)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return fmt.Errorf("%w: product=%s", ErrNotFound, l.id)
			}
		}
		return nil
	})
}

func (r *PGInventoryRepository) CheckStock(ctx context.Context, productID string, quantity int) (*models.StockCheckResult, error) {
	m, err := r.BatchGet(ctx, []string{productID})
	if err != nil {
		return nil, err
	}
	res := &models.StockCheckResult{ProductID: productID, Requested: quantity}
	if inv, ok := m[productID]; ok {
		res.Available, res.Reserved, res.IsSufficient = inv.Available, inv.Reserved, inv.Available >= quantity
	}
	return res, nil
}

// ListAll pages by product_id. ponytail: OFFSET paging on an admin-only list;
// keyset on product_id if the table grows large.
func (r *PGInventoryRepository) ListAll(ctx context.Context, limit, offset int) ([]models.Inventory, error) {
	var rows []pgInventory
	if err := r.db.WithContext(ctx).
		Raw("SELECT "+inventoryCols+" FROM catalog.inventory ORDER BY product_id LIMIT ? OFFSET ?", limit, offset).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]models.Inventory, len(rows))
	for i, row := range rows {
		out[i] = *row.model()
	}
	return out, nil
}
```

- [ ] **Step 6: Update the model, service, and service test**

`inventory/models/inventory.go` — replace the `Inventory` struct (drop `dynamodbav` tags and the DynamoDB comment):

```go
// Inventory represents the stock details of a product.
type Inventory struct {
	ProductID         string         `json:"product_id"`
	Available         int            `json:"available"`
	Reserved          int            `json:"reserved"`
	Threshold         int            `json:"threshold"`
	OrderReservations map[string]int `json:"order_reservations,omitempty"`
	UpdatedAt         time.Time      `json:"updated_at"`
}
```

`inventory/services/inventory_service.go`:
- Remove the import `"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"`, and `errors` / `time` if they become unused (let the compiler decide).
- Replace `SetStock` with:

```go
// SetStock adds req.Available to the product's stock (creating the record if
// needed) and overwrites the threshold, atomically.
func (s *InventoryService) SetStock(ctx context.Context, req *models.SetStockRequest) (*models.Inventory, error) {
	inv, err := s.repo.AddStock(ctx, req.ProductID, req.Available, req.Threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to set stock: %w", err)
	}
	log.Printf("[InventoryService] Stock set for product=%s available=%d (+%d) threshold=%d reserved=%d",
		req.ProductID, inv.Available, req.Available, inv.Threshold, inv.Reserved)
	return inv, nil
}
```
- Replace `ListAllStock`'s body after the clamps (delete the cursor loop and the `limit`/`startKey` vars):

```go
	items, err := s.repo.ListAll(ctx, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to list inventory: %w", err)
	}
	return items, nil
```
and update its doc comment (drop the "Walks DynamoDB Scan cursors" sentences). Also change the `CheckStock` doc comment "batched DynamoDB read" → "batched read". `UpdateStock` keeps passing its `updates` map unchanged (the repo ignores `updated_at`).

`inventory/services/inventory_service_test.go`:
- Remove the `dynamodb/types` import.
- Delete the `Set` mock method; add:

```go
func (m *MockInventoryRepository) AddStock(ctx context.Context, productID string, delta, threshold int) (*models.Inventory, error) {
	args := m.Called(ctx, productID, delta, threshold)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Inventory), args.Error(1)
}
```
- Replace the `ListAll` mock method with:

```go
func (m *MockInventoryRepository) ListAll(ctx context.Context, limit, offset int) ([]models.Inventory, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Inventory), args.Error(1)
}
```
- Append:

```go
func TestSetStock_UpsertsAtomicallyViaRepo(t *testing.T) {
	repo := new(MockInventoryRepository)
	service := NewInventoryService(repo, nil)
	want := &models.Inventory{ProductID: "p1", Available: 15, Threshold: 3}
	repo.On("AddStock", mock.Anything, "p1", 5, 3).Return(want, nil)

	got, err := service.SetStock(context.Background(), &models.SetStockRequest{ProductID: "p1", Available: 5, Threshold: 3})

	assert.NoError(t, err)
	assert.Equal(t, want, got)
	repo.AssertExpectations(t)
}

func TestListAllStock_PageBecomesOffset(t *testing.T) {
	repo := new(MockInventoryRepository)
	service := NewInventoryService(repo, nil)
	repo.On("ListAll", mock.Anything, 20, 40).Return([]models.Inventory{{ProductID: "p"}}, nil)

	items, err := service.ListAllStock(context.Background(), 3, 20)

	assert.NoError(t, err)
	assert.Len(t, items, 1)
	repo.AssertExpectations(t)
}
```

- [ ] **Step 7: Cut `main.go` over and add the reservation purger**

In `main.go`:
- Replace `inventoryRepo := inventoryrepository.NewDynamoInventoryRepository(ddbClient, cfg.DDBTableInventory)` with `inventoryRepo := inventoryrepository.NewPGInventoryRepository(gdb)`.
- Delete `ddbClient := dynamodb.NewFromConfig(awsCfg)` and the import `"github.com/aws/aws-sdk-go-v2/service/dynamodb"` (now unused).
- After the services are constructed, add the retention purge (CLAUDE.md convention 6: owners purge unbounded tables):

```go
	purgeCtx, stopPurge := context.WithCancel(context.Background())
	defer stopPurge()
	commondb.StartPurger(purgeCtx, gdb, time.Hour,
		commondb.PurgeJob{Table: "catalog.stock_reservations", Where: "status <> 'reserved' AND updated_at < now() - interval '30 days'"})
```

- [ ] **Step 8: Extend the seed converter with inventory**

Append to `seed_catalog_postgres.py`:

```python
print("-- inventory")
for i in rows("inventory"):
    if i["id"] not in live_prods:
        continue
    print("INSERT INTO catalog.inventory (product_id, available, reserved, threshold) VALUES "
          f"({q(i['id'])}, {i.get('available', 0)}, {i.get('reserved', 0)}, {i.get('threshold', 0)}) ON CONFLICT (product_id) DO NOTHING;")
```
(Dynamo `order_reservations` maps are dropped on purpose: the seed has none; for a real export with open reservations, convert them to `stock_reservations` rows by hand.)

Re-run `DATABASE_URL="$TEST_DATABASE_URL" ./scripts/seed_catalog.sh` and check `select count(*) from catalog.inventory` → 125.

- [ ] **Step 9: Run everything**

```bash
cd backend/services/catalog-service
go build ./... && go vet ./... && go test ./...
go test -race -count=1 ./inventory/... -run 'PGInventory|MergeItems|SetStock|ListAllStock|Reserve'
```
Expected: all PASS, including the concurrency tests under `-race`. If `TestPGInventory_OppositeOrderMultiProductReservesDoNotDeadlock` ever reports `deadlock detected`, the sort order in `mergeItems` is the culprit.

- [ ] **Step 10: End-to-end smoke through the stack**

```bash
cd backend
docker compose -f docker-compose.yml -f docker-compose.localstack.yml -f docker-compose.dev-ports.yml up -d --build catalog-service
PID=00000000-0000-4000-8000-000000000001
curl -s localhost:8082/inventory/$PID
curl -s -XPOST localhost:8082/inventory/reserve -H "X-Internal-Token: $INTERNAL_SERVICE_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"order_id\":\"smoke-1\",\"items\":[{\"product_id\":\"$PID\",\"quantity\":2}]}"
curl -s localhost:8082/inventory/$PID     # available -2, reserved +2, order_reservations {"smoke-1":2}
curl -s -XPOST localhost:8082/inventory/release -H "X-Internal-Token: $INTERNAL_SERVICE_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"order_id\":\"smoke-1\",\"items\":[{\"product_id\":\"$PID\",\"quantity\":2}]}"
curl -s localhost:8082/inventory/$PID     # back to the original numbers
```
Expected: numbers move as commented. (If the route needs a different auth header, use whatever `inventory/middleware/internal_auth.go` expects.)

- [ ] **Step 11: Commit**

```bash
git add backend/services/catalog-service backend/scripts/seed_catalog_postgres.py
git commit -m "feat(catalog): serve inventory from Postgres with transactional reservations"
```

---

### Task 8: Remove DynamoDB

**Files:** deletions and edits listed per step. Work from this checklist; finish with the grep gate.

- [ ] **Step 1: Check who imports `pkg/dynamodb` before deleting**

```bash
cd /Users/yashrajoria/ShopSwift/E-Commerce-backend/backend
grep -rn "pkg/dynamodb" --include='*.go' --include='go.mod' --include='go.work' . | grep -v node_modules
```
Expected: no importer left in `services/`. If one exists, port it first.

- [ ] **Step 2: Delete Dynamo code in catalog-service**

```bash
cd services/catalog-service
git rm repository/dynamo_adapter.go repository/dynamo_category_adapter.go repository/product_categories.go repository/product_categories_test.go
cd ../.. && git rm -r pkg/dynamodb
```
In `config.go` delete the four `DDBTable*` fields, their `os.Getenv` loads and their default-value `if` blocks. In `models/products.go` and `models/category.go` change the comment "Persistence is DynamoDB (see repository adapters)" to "Persistence is Postgres (schema catalog)".

- [ ] **Step 3: Delete the init tool and Dynamo process wiring**

```bash
cd /Users/yashrajoria/ShopSwift/E-Commerce-backend/backend
git rm -r tools/init-dynamo
```
- `go.work`: remove the `./tools/init-dynamo` line.
- `../start.sh` (the Render single-container entrypoint): delete the whole "Starting embedded DynamoDB" block (java `DynamoDBLocal.jar` launch, `sleep 2`, `/app/init-dynamo`) and the `# Route AWS SDK to embedded local DynamoDB` exports (`USE_LOCALSTACK=true`, `LOCALSTACK_ENDPOINT=http://localhost:8000`). That endpoint pointed *every* AWS client (S3, SNS) at DynamoDB Local, so those calls were already failing in the hosted deploy; leaving it would keep sending S3/SNS traffic to a dead port. Keep `AWS_USE_SECRETS=false`, `CLOUDWATCH_ENABLED=false` and the SQS-disabled exports as they are. The "Ensure GORM creates tables on Supabase" comment stays.
- `../Dockerfile`: the runtime stage is `FROM amazon/dynamodb-local:latest` (a Java image). Replace it with a slim base and drop the `init-dynamo` build line:

```dockerfile
FROM alpine:3.20
RUN apk add --no-cache bash ca-certificates tzdata
WORKDIR /app
```
  keeping the existing `COPY --from=builder /out/ /app/`, templates copy, `start.sh` copy, `chmod`, `EXPOSE 8080` and `ENTRYPOINT`. Delete `RUN CGO_ENABLED=0 … -o /out/init-dynamo ./tools/init-dynamo`, the `COPY backend/tools ./tools` line only if nothing else under `backend/tools` is built (`tools/loadtest` is not built here, so it can go), and fix the "5 Go services + init-dynamo" comment. Removing the JVM also frees memory on a small Render instance. Verify the image builds and boots: `docker build -t shopswift-test . && docker run --rm -e PORT=8080 shopswift-test` (it will fail to reach Postgres without env, but must get past binary startup; for a real check run it with the Supabase env).
- `docker-compose.yml`: delete the four `DDB_TABLE_*` lines from catalog-service. `docker-compose.localstack.yml`: remove any `dynamodb` entry from `SERVICES=`.
- `localstack/init/ready.d/10-bootstrap-resources.sh`: delete the DynamoDB section (header at line 65 through the last `create_*_table` function, ~line 200) and the calls to those functions at the bottom of the file. Keep S3/SNS/SQS.
- `scripts/dev-up.sh` line 4 comment: `SNS/SQS/DDB/S3` → `SNS/SQS/S3`.

- [ ] **Step 4: Replace Dynamo seed scripts**

```bash
cd scripts
git mv seed_dynamodb_worker.py seed_catalog_images.py
git rm seed_dynamodb_full.py seed_demo_data.sh seed_product.sh
```
Edit `seed_catalog_images.py`: delete the `dynamodb = boto3.client(...)` line, change the data path to `dynamodb_data.json` relative to the script (`os.path.join(os.path.dirname(__file__), "dynamodb_data.json")`, add `import os`), delete the unused `categories_items`/`products_items`/`inventory_items`/`product_categories_items` assignments (keep `svg_list`), and delete everything from `def batch_write` to the end. In `generate_all_seeds.py` delete `build_dynamodb_worker` (function around line 290) and the block near line 898 that writes `seed_dynamodb_worker.py` (keep the `dynamodb_data.json` output; leave the filename). `seed_postgres_data.sh` mentions `seed_demo_data.sh` in two comments — change them to `seed_catalog.sh`. Its hard-coded product IDs come from the same JSON (`…0001`–`…0037` prefix), so run `seed_catalog.sh` first.

- [ ] **Step 5: Terraform / AWS**

In `infrastructure/aws/terraform/`: remove the `aws_dynamodb_table` resources from `resources.tf`, the DynamoDB policy statements from `iam.tf`, the table outputs from `outputs.tf`, the table-name variables from `variables.tf`. Also edit `infrastructure/aws/tf-policy.json`, `populate_env_from_terraform.sh`, `seed_data.sh` to drop Dynamo references. Run:

```bash
cd infrastructure/aws/terraform && terraform fmt && terraform validate
```
Do NOT `terraform apply` here. This Terraform describes AWS resources that were never created (you ran LocalStack), so it's config cleanup only; if you've never applied it, there is nothing to destroy.

- [ ] **Step 6: Drop dependencies**

```bash
cd /Users/yashrajoria/ShopSwift/E-Commerce-backend/backend/services/catalog-service
go mod tidy && go build ./... && go vet ./... && go test ./...
grep -n dynamodb go.mod || echo "no dynamodb requirement left"
```
Expected: tests PASS; no dynamodb line (`aws-sdk-go-v2/service/dynamodb` and `attributevalue` gone). `cd ../.. && go build ./...` also passes at workspace level (`go.work.sum` may change; commit it).

- [ ] **Step 7: Docs**

- `CLAUDE.md` (root): stack line, service table (catalog: `PostgreSQL (catalog schema) + Redis + S3`), seed commands (`./scripts/seed_catalog.sh`, `python3 scripts/seed_catalog_images.py`; delete the `--sync-*` lines), section 4.5 ownership (`catalog-service` owns schema `catalog`: `categories`, `products`, `product_categories`, `inventory`, `stock_reservations`; reservations idempotent via `(order_id, product_id)` PK + status), delete the DynamoDB env block (`DDB_TABLE_*`).
- `backend/CLAUDE.md`: quick-commands seed line, convention 4 (`ClientRequestToken` → "reservation rows keyed by order"), convention 5 (mention catalog SQL migrations and the role script).
- `SERVICES_AND_DATABASES.md`: rewrite §3.2 (DynamoDB tables) as the `catalog` schema tables; fix the catalog service header, diagram labels (`DynamoDB / S3 / Redis` → `Postgres / S3 / Redis`), seed script description (line ~541).
- `MICROSERVICE_ARCHITECTURE.md`, `README.md`, `services/catalog-service/README.md`: replace Dynamo mentions; seed command at `MICROSERVICE_ARCHITECTURE.md:272`.
- `ROADMAP.md` line ~85: remove "removing DynamoDB" from the not-worth-it list and add a done phase: "Catalog moved from DynamoDB to Postgres (`catalog` schema, `catalog_svc` role, transactional inventory reservations)".
- `backend/docs/openapi.yaml`: grep for Dynamo wording in descriptions and fix (no path/schema changes).
- `backend/.env.example`: remove the `DDB_*` entries and the Dynamo comment; add `CATALOG_DB_USER=` / `CATALOG_DB_PASSWORD=` (blank = use `postgres` locally).

- [ ] **Step 8: Grep gate**

```bash
cd /Users/yashrajoria/ShopSwift/E-Commerce-backend
grep -rIil dynamo . --exclude-dir=node_modules --exclude-dir=.git --exclude-dir=graphify-out --exclude-dir=.build --exclude-dir=.tokensave --exclude-dir=.code-review-graph --exclude=go.sum --exclude=go.work.sum
```
Expected: only `ROADMAP.md` (history), this plan, and `backend/scripts/dynamodb_data.json` (filename of the seed data; its content is DynamoDB-typed JSON by design). Anything else gets fixed.

Final verification:

```bash
cd backend && for m in services/catalog-service services/order-service services/identity-service pkg/common; do (cd $m && go build ./...) || echo "FAIL $m"; done
(cd services/catalog-service && go test -race ./...) && docker compose -f docker-compose.yml -f docker-compose.localstack.yml config -q && echo OK
```
Plus a full local stack smoke: `./scripts/dev-up.sh`, `./scripts/seed_catalog.sh`, `./scripts/seed_postgres_data.sh`, browse storefront products, add to cart, checkout through to payment (exercises reserve → confirm).

- [ ] **Step 9: Commit**

```bash
git status --short   # review: stage ONLY the files this task touched
git add <each file or directory edited/deleted in Steps 2–7, listed explicitly>
git commit -m "chore(catalog): remove DynamoDB code, infra, scripts and docs"
```
(Never `git add -A`: the tree holds unrelated uncommitted work. `git rm` already stages deletions; `git add` the edited files by path.)

---

### Task 9: Production rollout and rollback

No code. Run these in order against production (Render + Supabase). Everything in the plan is deployed **once**, after Task 8, because Tasks 5 and 7 together remove the last Dynamo reads.

> **Merging to `main` auto-deploys.** `.github/workflows/ci-main.yml` job `deploy-render` fires on every push to `main` and does not wait for the `migrations` job. Complete Steps 1–4 (backup, migrations, role, data) and set the Render env vars from Step 5 **before** merging, or the new catalog-service will boot against a database with no `catalog` schema and fail readiness. (Optional hardening: add `migrations` to that job's `needs`.)

- [ ] **Step 1: Back up Postgres**

`./scripts/backup_postgres.sh` (writes to `backend/backups/`) or Supabase Dashboard → Database → Backups. Confirm the backup exists before continuing.

- [ ] **Step 2: Apply migrations 000015 and 000016 to Supabase**

Use the **direct or session-mode** connection string (DDL does not belong behind the transaction pooler):

```bash
cd backend
DATABASE_URL='postgres://postgres:<pw>@<host>:5432/postgres?sslmode=require' ./scripts/migrate.sh up
```
(Per backend CLAUDE.md convention 5: production runs with `ALLOW_AUTO_MIGRATE=true` and no migrate step, so this is manual.) Verify: `select table_name from information_schema.tables where table_schema='catalog'` lists 5 tables.

- [ ] **Step 3: Create the service role**

```bash
psql "$ADMIN_URL" -v ON_ERROR_STOP=1 -v catalog_password='<generate a long random password>' -f backend/infrastructure/postgres/catalog_role.sql
```
Store the password in Render's secret env, not in git. Check the Supabase Dashboard → Settings → API → Exposed schemas does not include `catalog`.

- [ ] **Step 4: Load data**

Reseed: `DATABASE_URL=$ADMIN_URL ./backend/scripts/seed_catalog.sh`. Or, if real data was exported (Task 5 Step 6), run the converter on `/tmp/real_data.json` and compare row counts with the Dynamo `COUNT` scans.

- [ ] **Step 5: Configure and deploy catalog-service**

On Render set: `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER=catalog_svc`, `POSTGRES_PASSWORD=<from step 3>`, `POSTGRES_DB`, `POSTGRES_SSLMODE=require`. Remove all `DDB_TABLE_*` vars. Deploy the `feat/catalog-postgres` branch (after review/merge).

- [ ] **Step 6: Smoke test production**

`GET /health/ready` → `ready`; list/search products; `GET /categories`; place a test order end to end (reserve → confirm), then cancel one (release) and check `GET /inventory/:id` numbers; check Supabase logs for errors; check pool usage: `select count(*), usename from pg_stat_activity group by 2` (catalog_svc ≤ 10).

- [ ] **Step 7: Rollback plan**

There is no AWS DynamoDB to fall back to: the old hosted catalog was an in-memory DynamoDB Local that reseeded on every start, so no data lives there. Rollback = redeploy the previous Render release; it rebuilds its own in-memory catalog from `init-dynamo`. Catalog rows written to Postgres after cutover (new products, stock changes) stay in Postgres and are not visible to the old release. Because the migrations only add a `catalog` schema, leaving them applied is harmless.

- [ ] **Step 8: Known gaps this migration does NOT fix**

Hosted today, S3 (product image upload/presign), SNS publish (cart checkout → `checkout.requested`) and the SQS consumers (`ENABLE_SQS_CONSUMER=false`) have no real backend, so those flows were already unavailable on Render. Moving the catalog to Postgres does not change that. They need their own plans: S3 → Cloudflare R2 / Supabase Storage (S3-compatible endpoint change), SNS/SQS → a Postgres-backed queue.

---

## Self-Review

**Spec coverage:** all four tables migrated (Tasks 1, 6); microservice boundary enforced by schema + role (Tasks 1, 2); best practices applied and listed in Global Constraints (partial/composite/FK indexes, `numeric`, `timestamptz`, upserts, consistent lock order, short transactions, batch inserts, least privilege, pool caps, retention purger); repository interfaces preserved so services/controllers are untouched; reservation idempotency replaces `ClientRequestToken`; seeding, CI, compose, Terraform, docs and prod rollout covered; Dynamo removal gated by a grep check.

**Placeholder scan:** code is given for every Go and SQL file. Task 8 edits to existing shell/Terraform/Docker files are specified by file, line region and exact deletion/replacement because those files are edited in place, not rewritten; each is verified by the grep gate and the build/compose checks.

**Type consistency:** `NewPGCategoryRepo`, `NewPGProductRepo`, `NewPGInventoryRepository`; `productWhere`, `jsonArray`, `uuidStrings`, `errRecordNotFound`, `mergeItems`/`stockLine{id, qty}`; `AddStock(ctx, productID string, delta, threshold int)`; `ListAll(ctx, limit, offset int)`; sentinel errors `ErrNotFound/ErrInsufficientStock/ErrDuplicateReservation/ErrNoReservation` — used identically in the interface (Task 7 Step 4), implementation (Step 5), tests (Step 2) and mocks (Step 6).
