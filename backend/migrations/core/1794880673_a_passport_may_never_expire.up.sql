-- A passport minted by hand may never expire; a null expiry means exactly that.
SET LOCAL lock_timeout = '3s';
ALTER TABLE passport ALTER COLUMN expires_at DROP NOT NULL;
