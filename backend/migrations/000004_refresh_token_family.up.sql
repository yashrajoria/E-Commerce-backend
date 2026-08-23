-- Track refresh token lineage so a reused (already-rotated) token can be
-- detected as theft and the whole family revoked, not just the one token.

ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS family_id uuid;

-- Existing rows have no lineage yet; seed each with its own id so they are
-- still individually revocable until they naturally expire/rotate.
UPDATE refresh_tokens SET family_id = id WHERE family_id IS NULL;

ALTER TABLE refresh_tokens ALTER COLUMN family_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family_id ON refresh_tokens (family_id);
