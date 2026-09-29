-- Payment-service outbox, mirroring order-service's outbox_events (000007).
-- Fixes the dual-write gap where publishPaymentEvent published to SNS
-- directly after the payment status UPDATE with no transactional guarantee.
CREATE TABLE IF NOT EXISTS payment_outbox_events (
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
    CONSTRAINT chk_payment_outbox_events_status CHECK (status IN ('pending', 'processing', 'published', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_payment_outbox_events_claimable
    ON payment_outbox_events (available_at, created_at)
    WHERE status IN ('pending', 'processing');

CREATE INDEX IF NOT EXISTS idx_payment_outbox_events_aggregate
    ON payment_outbox_events (aggregate_type, aggregate_id);
