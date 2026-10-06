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
