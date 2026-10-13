-- Go's int type maps to PostgreSQL bigint with the current GORM configuration.
-- Keep migrated schemas aligned with the model types used by AutoMigrate.

ALTER TABLE users
    ALTER COLUMN verification_attempts TYPE BIGINT,
    ALTER COLUMN login_attempts TYPE BIGINT;

ALTER TABLE coupons
    ALTER COLUMN usage_limit TYPE BIGINT,
    ALTER COLUMN used_count TYPE BIGINT;

ALTER TABLE outbox_events
    ALTER COLUMN attempts TYPE BIGINT;

ALTER TABLE payment_outbox_events
    ALTER COLUMN attempts TYPE BIGINT;
