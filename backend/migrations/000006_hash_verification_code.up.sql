-- Verification codes are now stored as a SHA-256 hex hash (64 chars), not
-- plaintext, so a DB read alone can't hand over a live code. Any code issued
-- before this migration stops matching (functionally equivalent to expiry —
-- the user just requests a new one via resend-verification).
ALTER TABLE users ALTER COLUMN verification_code TYPE VARCHAR(64);
UPDATE users SET verification_code = '' WHERE verification_code IS NOT NULL AND length(verification_code) < 64;
