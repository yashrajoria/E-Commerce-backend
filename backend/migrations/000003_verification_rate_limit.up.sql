-- Rate-limit email verification code attempts (brute-force protection).

ALTER TABLE users ADD COLUMN IF NOT EXISTS verification_attempts INT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS verification_locked_until TIMESTAMPTZ;
