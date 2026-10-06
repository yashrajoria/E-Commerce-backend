DROP TRIGGER IF EXISTS payment_matches_order ON payments;
DROP FUNCTION IF EXISTS trg_payment_matches_order();

DROP TRIGGER IF EXISTS order_status_history ON orders;
DROP FUNCTION IF EXISTS trg_order_status_history();
DROP TABLE IF EXISTS order_status_history;

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['users','addresses','orders','payments','coupons','shipments',
                           'agent_audit_log','outbox_events','payment_outbox_events']
  LOOP
    CONTINUE WHEN to_regclass(t) IS NULL;
    EXECUTE format('DROP TRIGGER IF EXISTS set_updated_at ON %I', t);
  END LOOP;
END $$;
DROP FUNCTION IF EXISTS trg_set_updated_at();
-- orders.currency stays: the Go model writes it.
