CREATE TABLE IF NOT EXISTS agent_audit_log (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  request_id uuid NOT NULL,
  user_id uuid,
  prompt text,
  tool varchar(64) NOT NULL,
  arguments jsonb NOT NULL DEFAULT '{}'::jsonb,
  mutating boolean NOT NULL DEFAULT false,
  status varchar(32) NOT NULL DEFAULT 'completed',
  confirmed_by uuid,
  confirmed_at timestamptz,
  result jsonb,
  error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_agent_audit_log_request_id ON agent_audit_log (request_id);
CREATE INDEX IF NOT EXISTS idx_agent_audit_log_user_id ON agent_audit_log (user_id);
CREATE INDEX IF NOT EXISTS idx_agent_audit_log_status ON agent_audit_log (status);
