ALTER TABLE orders DROP COLUMN IF EXISTS refunded_amount;
ALTER TABLE payments DROP COLUMN IF EXISTS refunded_amount;
DROP TABLE IF EXISTS order_payment_allocations;
DROP TABLE IF EXISTS store_credit_ledger;
DROP TABLE IF EXISTS store_credit_accounts;
