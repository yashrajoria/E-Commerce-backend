-- Migration 000017: Postgres-backed message queue (replacing AWS SQS & SNS)

CREATE TABLE IF NOT EXISTS message_queues (
    id BIGSERIAL PRIMARY KEY,
    queue_name TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    visible_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 5,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_message_queues_poll
ON message_queues (queue_name, status, visible_at)
WHERE status IN ('pending', 'processing');

CREATE INDEX IF NOT EXISTS idx_message_queues_cleanup
ON message_queues (status, updated_at)
WHERE status = 'completed';
