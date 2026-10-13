ALTER TABLE payment_outbox_events
    ALTER COLUMN attempts TYPE INTEGER;

ALTER TABLE outbox_events
    ALTER COLUMN attempts TYPE INTEGER;

ALTER TABLE coupons
    ALTER COLUMN usage_limit TYPE INTEGER,
    ALTER COLUMN used_count TYPE INTEGER;

ALTER TABLE users
    ALTER COLUMN verification_attempts TYPE INTEGER,
    ALTER COLUMN login_attempts TYPE INTEGER;
