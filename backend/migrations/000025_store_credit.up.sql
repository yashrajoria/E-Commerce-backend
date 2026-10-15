-- Milestone 1: Store-scoped Store Credit & Payment Allocation
-- Accounts maintain integer minor-unit cached balances (available_amount, held_amount).
-- The ledger is immutable and the single source of truth; all mutations execute atomically.
-- Expiration is intentionally omitted for MVP.

CREATE TABLE IF NOT EXISTS store_credit_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    currency varchar(10) NOT NULL DEFAULT 'GBP',
    available_amount bigint NOT NULL DEFAULT 0 CHECK (available_amount >= 0),
    held_amount bigint NOT NULL DEFAULT 0 CHECK (held_amount >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (store_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_store_credit_accounts_store_user 
    ON store_credit_accounts(store_id, user_id);

CREATE TABLE IF NOT EXISTS store_credit_ledger (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL REFERENCES store_credit_accounts(id) ON DELETE CASCADE,
    entry_type varchar(32) NOT NULL, -- 'grant', 'debit', 'hold', 'capture', 'release', 'refund'
    amount bigint NOT NULL DEFAULT 0, -- net total fund delta (available_delta + held_delta)
    available_delta bigint NOT NULL DEFAULT 0,
    held_delta bigint NOT NULL DEFAULT 0,
    currency varchar(10) NOT NULL,
    order_id uuid REFERENCES orders(id),
    idempotency_key varchar(128) NOT NULL,
    reason text NOT NULL DEFAULT '',
    metadata jsonb NOT NULL DEFAULT '{}',
    created_by uuid REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (account_id, idempotency_key, entry_type)
);

CREATE INDEX IF NOT EXISTS idx_store_credit_ledger_account 
    ON store_credit_ledger(account_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_store_credit_ledger_order 
    ON store_credit_ledger(order_id) 
    WHERE order_id IS NOT NULL;

-- Payment allocations table for mixed payment methods (Store Credit + Stripe) & partial refunds
CREATE TABLE IF NOT EXISTS order_payment_allocations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    payment_method varchar(32) NOT NULL, -- 'store_credit', 'stripe'
    amount bigint NOT NULL CHECK (amount > 0),
    currency varchar(10) NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'allocated', -- 'allocated', 'captured', 'refunded', 'released'
    refunded_amount bigint NOT NULL DEFAULT 0 CHECK (refunded_amount >= 0),
    reference_id varchar(128) NOT NULL DEFAULT '',
    metadata jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (refunded_amount <= amount)
);

CREATE INDEX IF NOT EXISTS idx_order_payment_allocations_order 
    ON order_payment_allocations(order_id);

ALTER TABLE orders ADD COLUMN IF NOT EXISTS refunded_amount bigint NOT NULL DEFAULT 0;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS refunded_amount bigint NOT NULL DEFAULT 0;
