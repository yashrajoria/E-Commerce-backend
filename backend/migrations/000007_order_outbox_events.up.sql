-- Phase 1 reliability schema.
--
-- The outbox makes order creation and downstream event creation atomic. The
-- publisher later claims these rows and delivers them at least once to SQS or
-- SNS. Lease columns allow another publisher to recover abandoned work.
CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id UUID NOT NULL,
    event_type VARCHAR(150) NOT NULL,
    destination_type VARCHAR(20) NOT NULL,
    destination TEXT NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_owner VARCHAR(128),
    lease_expires_at TIMESTAMPTZ,
    claimed_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_outbox_events_status CHECK (status IN ('pending', 'processing', 'published', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_outbox_events_claimable
    ON outbox_events (available_at, created_at)
    WHERE status IN ('pending', 'processing');

CREATE INDEX IF NOT EXISTS idx_outbox_events_aggregate
    ON outbox_events (aggregate_type, aggregate_id);

-- Payment requests use a stable event ID so a redelivered message cannot
-- create a second payment or Stripe checkout session.
ALTER TABLE payments ADD COLUMN IF NOT EXISTS event_id VARCHAR(128);

CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_event_id
    ON payments (event_id);

-- Notification consumers claim an event before sending external messages.
-- A primary key makes the claim idempotent across concurrent redeliveries.
CREATE TABLE IF NOT EXISTS notification_events (
    event_id VARCHAR(128) PRIMARY KEY,
    status VARCHAR(32) NOT NULL,
    processed_at TIMESTAMPTZ
);