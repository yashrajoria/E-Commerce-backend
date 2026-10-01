-- Rate-limit password login attempts (brute-force protection), mirroring
-- the verification-code lockout added in 000003.

ALTER TABLE users ADD COLUMN IF NOT EXISTS login_attempts INT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS login_locked_until TIMESTAMPTZ;
