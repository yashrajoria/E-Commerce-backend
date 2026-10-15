-- 000024_admin_portal_tables.down.sql
DROP TABLE IF EXISTS campaigns CASCADE;
DROP TABLE IF EXISTS activity_logs CASCADE;
DROP TABLE IF EXISTS reviews CASCADE;
DROP TABLE IF EXISTS returns CASCADE;
DROP TABLE IF EXISTS support_tickets CASCADE;

ALTER TABLE shipments DROP COLUMN IF EXISTS destination;
ALTER TABLE shipments DROP COLUMN IF EXISTS estimated_delivery;
ALTER TABLE shipments DROP COLUMN IF EXISTS carrier;

