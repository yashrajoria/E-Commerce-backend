-- +migrate Up
CREATE TABLE IF NOT EXISTS shipments (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id         VARCHAR(255) NOT NULL,
    user_id          VARCHAR(255) NOT NULL,
    carrier          VARCHAR(100),
    service_level    VARCHAR(100),
    tracking_code    VARCHAR(255),
    label_url        TEXT,
    tracking_url     TEXT,
    shippo_object_id VARCHAR(255),
    status           VARCHAR(50) NOT NULL DEFAULT 'created',
    weight_kg        NUMERIC(10, 3) NOT NULL DEFAULT 0,
    origin_json      JSONB,
    destination_json JSONB,
    created_at       TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at       TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at       TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_shipments_order_id ON shipments(order_id);
CREATE INDEX IF NOT EXISTS idx_shipments_tracking_code ON shipments(tracking_code);
CREATE INDEX IF NOT EXISTS idx_shipments_user_id ON shipments(user_id);
CREATE INDEX IF NOT EXISTS idx_shipments_status ON shipments(status);
CREATE INDEX IF NOT EXISTS idx_shipments_deleted_at ON shipments(deleted_at);
