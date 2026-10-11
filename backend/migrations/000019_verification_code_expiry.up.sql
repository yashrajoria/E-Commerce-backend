-- Emailed verification codes expire; NULL (rows issued before this migration) = no expiry.
ALTER TABLE users ADD COLUMN IF NOT EXISTS verification_expires_at TIMESTAMPTZ;
